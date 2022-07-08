package network

import (
	"errors"
	"fmt"
	"net"

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

	vmNS netns.NsHandle

	vnetNSName string
	vnetNS     netns.NsHandle

	mode           string
	vlanID         int
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
		netlink:           nl,
		netioshim:         &netio.NetIO{},
		plClient:          plc,
		netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
	}

	return client
}

func (client *NativeEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	var err error
	log.Printf("Get VM namespace handle")
	vmNS, err := netns.Get()
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Save VM namespace: %s", vmNS)
	client.vmNS = vmNS

	log.Printf("Create the vnet namespace and switch to it")
	if client.vnetNS, err = netns.NewNamed(client.vnetNSName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Set current namespace to VM: %s", vmNS)
	err = netns.Set(vmNS)
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
	if err = vishnetlink.LinkAdd(link); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Move vlan link to vnet NS: %d", uintptr(client.vnetNS))
	if err = client.netlink.SetLinkNetNs(client.ethXVethName, uintptr(client.vnetNS)); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Create veth pair (automatically set to UP)")
	if err = client.netUtilsClient.CreateEndpoint(client.vnetVethName, client.containerVethName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Move vnetVethName into vnet namespace")
	if err = client.netlink.SetLinkNetNs(client.vnetVethName, uintptr(client.vnetNS)); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	//If there is a failure, delete the links
	defer func() {
		if err != nil {
			log.Printf("Switching NS to vnet")
			netns.Set(client.vnetNS)
			log.Printf("Failure detected, deleting links...")
			//Delete vnet <-> container
			if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
				log.Errorf("Deleting vnetVeth failed on addendpoint failure:%v", delErr)
			}
			//Delete eth0 <-> eth0.X
			if delErr := client.netlink.DeleteLink(client.ethXVethName); delErr != nil {
				log.Errorf("Deleting hostVeth failed on addendpoint failure:%v", delErr)
			}
			log.Printf("Switching NS to vm")
			netns.Set(client.vmNS)
		}
	}()
	log.Printf("Check that container veth exists.")
	containerIf, err := client.netioshim.GetNetworkInterfaceByName(client.containerVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.containerMac = containerIf.HardwareAddr

	log.Printf("Switch NS to vnet")
	netns.Set(client.vnetNS)

	currNS, err := netns.Get()
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

	log.Printf("Switch NS to vm")
	netns.Set(client.vmNS)

	return nil
}
func (client *NativeEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	//There are no rules to add here
	//Described as rules on ip addresses on the container interface

	return nil
}

// Helper that creates routing rules for the current NS which direct packets
// to the virtual gateway ip on linkToName device interface
func (client *NativeEndpointClient) AddDefaultRoutes(linkToName string) error {
	log.Printf("Add route for virtualgwip (ip route add 169.254.1.1/32 dev eth0)")
	virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
	routeInfo := RouteInfo{
		Dst:     *virtualGwNet,
		Scope:   netlink.RT_SCOPE_LINK,
		DevName: linkToName,
	}
	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}

	log.Printf("Add default route (ip route add default via 169.254.1.1 dev eth0)")
	_, defaultIPNet, _ := net.ParseCIDR(defaultGwCidr)
	dstIP := net.IPNet{IP: net.ParseIP(defaultGw), Mask: defaultIPNet.Mask}
	routeInfo = RouteInfo{
		Dst:     dstIP,
		Gw:      virtualGwIP,
		DevName: linkToName,
	}

	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}
	return nil
}

// Helper that creates arp entry for the current NS which maps the virtual
// gateway to destMac
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
		interfaceName, //What is the purpose of name?
		virtualGwNet.IP,
		hardwareAddr,
		false); err != nil {
		return fmt.Errorf("adding arp entry failed: %w", err)
	}
	return nil
}
func (client *NativeEndpointClient) DeleteEndpointRules(ep *endpoint) {

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
func (client *NativeEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	log.Printf("Setting NS to container path %d", uintptr(client.vnetNS))
	contNS, err := netns.GetFromPath(epInfo.NetNsPath)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	err = netns.Set(contNS)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Assign IPs to container veth interface")
	if err = client.netUtilsClient.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
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

	/* Ignore the resolv.conf for now
	log.Printf("Create resolv.conf for DNS")
	folder := fmt.Sprintf("/etc/netns/%s", client.vnetNSName)
	resolv := fmt.Sprintf("%s/resolv.conf", folder)
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		os.MkdirAll(folder, 0700) // Create your file
	}
	log.Printf("Writing to resolv.conf file %s , %s", epInfo.DNS.Servers, epInfo.DNS.Servers[0])
	data := []byte(fmt.Sprintf("nameserver %s", epInfo.DNS.Servers[0]))
	if err := os.WriteFile(resolv, data, 0644); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	*/

	currNS, err := netns.Get()
	log.Printf("Current NS before switch: %v.", currNS)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	// Open the vnet network namespace
	log.Printf("Opening vnetns %v.", fmt.Sprintf("/var/run/netns/%s", client.vnetNSName))
	ns, err := OpenNamespace(fmt.Sprintf("/var/run/netns/%s", client.vnetNSName))
	if err != nil {
		return err
	}
	defer ns.Close()
	// Enter the vnet network namespace
	log.Printf("Entering vnetns %v.", ns)
	if err := ns.Enter(); err != nil {
		return err
	}

	// Exit vnet network namespace
	defer func() {
		log.Printf("Exiting vnetns %v.", ns)
		if err := ns.Exit(); err != nil {
			log.Printf("Could not exit vnetns, err:%v.", err)
		}
	}()

	log.Printf("Current NS after switch: %v.", currNS)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Setting vnet loopback state to up")
	err = client.netlink.SetLinkState(loopbackIf, true)
	if err != nil {
		log.Printf("Failed to set loopback link state to up")
		return newErrorNativeEndpointClient(err.Error())
	}

	var routeInfoList []RouteInfo

	// Add route specifying which device the pod ip(s) are on
	for _, ipAddr := range epInfo.IPAddresses {
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
	log.Printf("Client data: ethX: %s, vnet: %s", client.ethXVethName, client.vnetVethName)

	log.Printf("Vnet NS add default/gateway routes (Assuming indempotent)")
	if err = client.AddDefaultRoutes(client.ethXVethName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Vnet NS add default ARP entry (Assuming indempotent)")
	if err = client.AddDefaultArp(client.ethXVethName, azureMac); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	log.Printf("Adding routes")
	if err := addRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	log.Printf("Return to container NS")
	err = netns.Set(contNS)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	return nil
}
func (client *NativeEndpointClient) DeleteEndpoints(ep *endpoint) error {
	return nil
}
