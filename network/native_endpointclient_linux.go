package network

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"

	vishnetlink "github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	azureMac   = "12:34:56:78:9a:bc"
	loopbackIf = "lo"
)

//Move somewhere else
type NetnsInterface interface {
	Get() (fileDescriptor uintptr, err error)
	GetFromName(name string) (fileDescriptor uintptr, err error)
	Set(fileDescriptor uintptr) (err error)
	NewNamed(name string) (fileDescriptor uintptr, err error)
}
type Netns struct{}

func NewNetns() *Netns {
	return &Netns{}
}
func (f *Netns) Get() (uintptr, error) {
	nsHandle, err := netns.Get()
	return uintptr(nsHandle), err
}
func (f *Netns) GetFromName(name string) (uintptr, error) {
	nsHandle, err := netns.GetFromName(name)
	return uintptr(nsHandle), err
}
func (f *Netns) Set(fileDescriptor uintptr) error {
	return netns.Set(netns.NsHandle(fileDescriptor))
}
func (f *Netns) NewNamed(name string) (uintptr, error) {
	nsHandle, err := netns.NewNamed(name)
	return uintptr(nsHandle), err
}

var ErrorMockNetns = errors.New("mock netns error")

func newErrorMockNetns(errStr string) error {
	return fmt.Errorf("%w : %s", ErrorMockNetns, errStr)
}

type MockNetns struct {
	failMethod  int
	failMessage string
}

func NewMockNetns(failMethod int, failMessage string) *MockNetns {
	return &MockNetns{
		failMethod:  failMethod,
		failMessage: failMessage,
	}
}
func (f *MockNetns) Get() (uintptr, error) {
	if f.failMethod == 1 {
		return 0, newErrorMockNetns(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) GetFromName(name string) (uintptr, error) {
	if f.failMethod == 2 {
		return 0, newErrorMockNetns(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) Set(handle uintptr) error {
	if f.failMethod == 3 {
		return newErrorMockNetns(f.failMessage)
	}
	return nil
}
func (f *MockNetns) NewNamed(name string) (uintptr, error) {
	if f.failMethod == 4 {
		return 0, newErrorMockNetns(f.failMessage)
	}
	return 1, nil
}

//End move somewhere else

var errorNativeEndpointClient = errors.New("NativeEndpointClient Error")

func newErrorNativeEndpointClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorNativeEndpointClient, errStr)
}

type NativeEndpointClient struct {
	eth0VethName      string //So like eth0
	ethXVethName      string //So like eth0.X
	vnetVethName      string //Peer is containerVethName
	containerVethName string //Peer is vnetVethName

	vnetMac      net.HardwareAddr
	containerMac net.HardwareAddr
	ethXMac      net.HardwareAddr

	vnetNSName           string
	vnetNSFileDescriptor uintptr

	mode           string
	vlanID         int
	netnsClient    NetnsInterface
	netlink        netlink.NetlinkInterface
	netioshim      netio.NetIOInterface
	plClient       platform.ExecClient
	netUtilsClient networkutils.NetworkUtils
}

func NewNativeEndpointClient(
	eth0VethName string,
	ethXVethName string,
	vnetVethName string,
	containerVethName string,
	vnetNSName string,
	mode string,
	vlanid int,
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
) *NativeEndpointClient {
	log.Printf("Create new native client: eth0:%s, ethX:%s, vnet:%s, cont:%s, id:%s",
		eth0VethName, ethXVethName, vnetVethName, containerVethName, vlanid)
	client := &NativeEndpointClient{
		eth0VethName:      eth0VethName,
		ethXVethName:      ethXVethName,
		vnetVethName:      vnetVethName,
		containerVethName: containerVethName,
		vnetNSName:        vnetNSName,
		mode:              mode,
		vlanID:            vlanid,
		netnsClient:       NewNetns(),
		netlink:           nl,
		netioshim:         &netio.NetIO{},
		plClient:          plc,
		netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
	}

	return client
}

// Adds interfaces to the vnet (created if not existing) and vm namespace
func (client *NativeEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	var err error
	err = client.PopulateClient(epInfo)
	if err != nil {
		return err
	}

	err = ExecuteInNS(client.vnetNSName, epInfo, client.PopulateVnet)
	if err != nil {
		return err
	}

	return nil
}

// Called from AddEndpoints, Namespace: VM
func (client *NativeEndpointClient) PopulateClient(epInfo *EndpointInfo) error {
	log.Printf("Get VM namespace handle")
	vmNS, err := client.netnsClient.Get()
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	//Mostly for reference
	returnedTo, err := GetCurrentThreadNamespace()
	if err != nil {
		log.Printf("Unable to get VM namespace: %v", err)
		return newErrorNativeEndpointClient(err.Error())
	} else {
		log.Printf("VM Namespace: %s", returnedTo.file.Name())
	}

	log.Printf("Checking if NS exists...")
	var existingErr error
	vnetNS, existingErr := client.netnsClient.GetFromName(client.vnetNSName)
	//If the ns does not exist, the below code will trigger to create it
	if existingErr != nil {
		if !strings.Contains(strings.ToLower(existingErr.Error()), "no such file or directory") {
			return newErrorNativeEndpointClient(existingErr.Error())
		} else {
			log.Printf("No existing NS detected. Creating the vnet namespace and switching to it")
			vnetNS, err = client.netnsClient.NewNamed(client.vnetNSName)
			if err != nil {
				return newErrorNativeEndpointClient(err.Error())
			}

		}
	}
	client.vnetNSFileDescriptor = uintptr(vnetNS)

	log.Printf("Set current namespace to VM: %s", vmNS)
	err = client.netnsClient.Set(vmNS)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Create the host vlan link after getting eth0: %s", client.eth0VethName)
	linkAttrs := vishnetlink.NewLinkAttrs()
	linkAttrs.Name = client.ethXVethName
	//Get parent interface index. Index is consistent across libraries.
	eth0, err := client.netioshim.GetNetworkInterfaceByName(client.eth0VethName)
	if err != nil {
		log.Printf("Failed to get interface: %s", client.eth0VethName)
		return newErrorNativeEndpointClient(err.Error())
	}
	//Set the peer
	linkAttrs.ParentIndex = eth0.Index
	link := &vishnetlink.Vlan{
		LinkAttrs: linkAttrs,
		VlanId:    client.vlanID,
	}
	log.Printf("Add link to VM NS (automatically set to UP)")
	// Attempting to create ethX
	existingErr = vishnetlink.LinkAdd(link)
	ethXCreated := true
	if existingErr != nil {
		if !strings.Contains(strings.ToLower(existingErr.Error()), "file exists") {
			return newErrorNativeEndpointClient(existingErr.Error())
		} else {
			log.Printf("eth0.X already exists")
			ethXCreated = false
		}
	}
	if ethXCreated {
		log.Printf("Move vlan link (ethX) to vnet NS: %d", uintptr(client.vnetNSFileDescriptor))
		if err = client.netlink.SetLinkNetNs(client.ethXVethName, uintptr(client.vnetNSFileDescriptor)); err != nil {
			log.Printf("Deleting ethX veth in VM NS due to addendpoint failure")
			if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
				log.Errorf("Deleting ethX veth failed on addendpoint failure:%v", delErr)
			}
			return newErrorNativeEndpointClient(err.Error())
		}
	}

	log.Printf("Create veth pair (automatically set to UP)")
	if err = client.netUtilsClient.CreateEndpoint(client.vnetVethName, client.containerVethName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Move vnetVethName into vnet namespace")
	if err = client.netlink.SetLinkNetNs(client.vnetVethName, uintptr(client.vnetNSFileDescriptor)); err != nil {
		log.Printf("Deleting vnet veth due to addendpoint failure")
		if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
			log.Errorf("Deleting vnet veth failed on addendpoint failure:%v", delErr)
		}
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Check that container veth exists.")
	containerIf, err := client.netioshim.GetNetworkInterfaceByName(client.containerVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.containerMac = containerIf.HardwareAddr
	return nil
}

// Called from AddEndpoints, Namespace: Vnet
func (client *NativeEndpointClient) PopulateVnet(epInfo *EndpointInfo) error {

	currNS, err := client.netnsClient.Get()
	log.Printf("Current NS after switch to vnet: %s", currNS)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Check that (eth0.X) exists")
	ethXVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.ethXVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.ethXMac = ethXVethIf.HardwareAddr

	log.Printf("Check that vnet veth exists")
	vnetVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.vnetVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.vnetMac = vnetVethIf.HardwareAddr
	return nil
}
func (client *NativeEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	//There are no rules to add here
	//Described as rules on ip addresses on the container interface

	return nil
}

func (client *NativeEndpointClient) DeleteEndpointRules(ep *endpoint) {
	//Never added any endpoint rules
}
func (client *NativeEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	log.Printf("Moving endpoint to container NS")
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	return nil
}
func (client *NativeEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	log.Printf("Setup container interface")
	if err := client.netUtilsClient.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.containerVethName = epInfo.IfName

	return nil
}

// Adds routes, arp entries, etc. to the vnet and container namespaces
func (client *NativeEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	err := client.ConfigureContainerInterfacesAndRoutesImpl(epInfo)
	if err != nil {
		return err
	}

	//Switch to vnet NS and call ConfigureVnetInterfacesAndRoutes
	err = ExecuteInNS(client.vnetNSName, epInfo, client.ConfigureVnetInterfacesAndRoutesImpl)
	return err
}

// Called from ConfigureContainerInterfacesAndRoutes, Namespace: Container
func (client *NativeEndpointClient) ConfigureContainerInterfacesAndRoutesImpl(epInfo *EndpointInfo) error {
	log.Printf("Assign IPs to container veth interface")
	if err := client.netUtilsClient.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Remove kernel subnet route automatically added by above")
	for _, ipAddr := range epInfo.IPAddresses {
		_, ipnet, _ := net.ParseCIDR(ipAddr.String())
		routeInfo := RouteInfo{
			Dst:      *ipnet,
			Scope:    netlink.RT_SCOPE_LINK,
			Protocol: netlink.RTPROT_KERNEL,
		}
		if err := deleteRoutes(client.netlink, client.netioshim, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
			return newErrorTransparentEndpointClient(err.Error())
		}
	}

	log.Printf("Container NS add route for virtual gateway ip")
	if err := client.AddDefaultRoutes(client.containerVethName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Container NS add arp entry")
	if err := client.AddDefaultArp(client.containerVethName, client.vnetMac.String()); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	return nil
}

// Called from ConfigureContainerInterfacesAndRoutes, Namespace: Vnet
func (client *NativeEndpointClient) ConfigureVnetInterfacesAndRoutesImpl(epInfo *EndpointInfo) error {
	log.Printf("Setting vnet loopback state to up")
	err := client.netlink.SetLinkState(loopbackIf, true)
	if err != nil {
		log.Printf("Failed to set loopback link state to up")
		return newErrorNativeEndpointClient(err.Error())
	}

	// Add route specifying which device the pod ip(s) are on
	routeInfoList := client.GetVnetRoutes(epInfo.IPAddresses)

	log.Printf("Client data: ethX: %s, vnet: %s", client.ethXVethName, client.vnetVethName)

	log.Printf("Vnet NS add default/gateway routes (Assuming indempotent)")
	if err = client.AddDefaultRoutes(client.ethXVethName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Vnet NS add default ARP entry (Assuming indempotent)")
	if err = client.AddDefaultArp(client.ethXVethName, azureMac); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Adding routes to vnet specific to this container")
	if err := addRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	// Return to ConfigureContainerInterfacesAndRoutes
	return err
}

// Helper that gets the routes in the vnet NS for a particular list of IP addresses
// Example: 192.168.0.4 dev <device which connects to NS with that IP> proto static
func (client *NativeEndpointClient) GetVnetRoutes(ipAddresses []net.IPNet) []RouteInfo {
	var routeInfoList []RouteInfo
	// Add route specifying which device the pod ip(s) are on
	for _, ipAddr := range ipAddresses {
		var (
			routeInfo RouteInfo
			ipNet     net.IPNet
		)

		if ipAddr.IP.To4() != nil {
			ipNet = net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)}
		} else {
			ipNet = net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(ipv6FullMask, ipv6Bits)}
		}
		log.Printf("[net] Native client adding route for the ip %v", ipNet.String())
		routeInfo.Dst = ipNet
		routeInfoList = append(routeInfoList, routeInfo)

	}
	return routeInfoList
}

// Helper that creates routing rules for the current NS which direct packets
// to the virtual gateway ip on linkToName device interface
// Route 1: 169.254.1.1 dev <linkToName>
// Route 2: default via 169.254.1.1 dev <linkToName>
func (client *NativeEndpointClient) AddDefaultRoutes(linkToName string) error {
	log.Printf("Add route for virtualgwip (ip route add 169.254.1.1/32 dev eth0)")
	virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
	routeInfo := RouteInfo{
		Dst:   *virtualGwNet,
		Scope: netlink.RT_SCOPE_LINK,
	}
	// Difference between interface name in addRoutes and DevName: in RouteInfo?
	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}

	log.Printf("Add default route (ip route add default via 169.254.1.1 dev eth0)")
	_, defaultIPNet, _ := net.ParseCIDR(defaultGwCidr)
	dstIP := net.IPNet{IP: net.ParseIP(defaultGw), Mask: defaultIPNet.Mask}
	routeInfo = RouteInfo{
		Dst: dstIP,
		Gw:  virtualGwIP,
	}

	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}
	return nil
}

// Helper that creates arp entry for the current NS which maps the virtual
// gateway to destMac on a particular interfaceName
// Example: (169.254.1.1) at 12:34:56:78:9a:bc [ether] PERM on <interfaceName>
func (client *NativeEndpointClient) AddDefaultArp(interfaceName string, destMac string) error {
	_, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
	// arp -s 169.254.1.1 12:34:56:78:9a:bc - add static arp entry for virtualgwip to hostveth interface mac
	log.Printf("[net] Adding static arp for IP address %v and MAC %v in namespace",
		virtualGwNet.String(), destMac)
	hardwareAddr, err := net.ParseMAC(destMac)
	if err != nil {
		return err
	}
	if err := client.netlink.AddOrRemoveStaticArp(netlink.ADD,
		interfaceName,
		virtualGwNet.IP,
		hardwareAddr,
		false); err != nil {
		return fmt.Errorf("adding arp entry failed: %w", err)
	}
	return nil
}
func (client *NativeEndpointClient) DeleteEndpoints(ep *endpoint) error {
	return ExecuteInNS(client.vnetNSName, ep, client.DeleteEndpointsImpl)
}
func (client *NativeEndpointClient) DeleteEndpointsImpl(ep *endpoint) error {
	log.Printf("Removing routes")
	routeInfoList := client.GetVnetRoutes(ep.IPAddresses)
	if err := deleteRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	return nil
}

// Helper function that allows executing a function with one parameter in a VM namespace
// Does not work for process namespaces
func ExecuteInNS[T any](nsName string, param *T, f func(param *T) error) error {
	// Current namespace
	returnedTo, err := GetCurrentThreadNamespace()
	if err != nil {
		log.Printf("[ExecuteInNS] Could not get NS we are in: %v", err)
	} else {
		log.Printf("[ExecuteInNS] In NS before switch: %s", returnedTo.file.Name())
	}

	// Open the network namespace
	log.Printf("[ExecuteInNS] Opening ns %v.", fmt.Sprintf("/var/run/netns/%s", nsName))
	ns, err := OpenNamespace(fmt.Sprintf("/var/run/netns/%s", nsName))
	if err != nil {
		return err
	}
	defer ns.Close()
	// Enter the network namespace
	log.Printf("[ExecuteInNS] Entering vnetns %s.", ns.file.Name())
	if err := ns.Enter(); err != nil {
		return err
	}

	// Exit network namespace
	defer func() {
		log.Printf("[ExecuteInNS] Exiting vnetns %s.", ns.file.Name())
		if err := ns.Exit(); err != nil {
			log.Printf("[ExecuteInNS] Could not exit ns, err:%v.", err)
		}
		returnedTo, err := GetCurrentThreadNamespace()
		if err != nil {
			log.Printf("[ExecuteInNS] Could not get NS we returned to: %v", err)
		} else {
			log.Printf("[ExecuteInNS] Returned to NS: %s", returnedTo.file.Name())
		}
	}()
	return f(param)
}
