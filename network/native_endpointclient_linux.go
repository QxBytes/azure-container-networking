package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"

	vishnetlink "github.com/vishvananda/netlink"
)

const (
	azureMac         = "12:34:56:78:9a:bc" // Packets leaving the VM should have this MAC
	loopbackIf       = "lo"                // The name of the loopback interface
	numDefaultRoutes = 2                   // VNET NS, when no containers use it, has this many routes
)

type NativeEndpointClient struct {
	eth0VethName      string //So like eth0
	ethXVethName      string //So like eth0.X
	vnetVethName      string //Peer is containerVethName
	containerVethName string //Peer is vnetVethName

	vnetMac      net.HardwareAddr
	containerMac net.HardwareAddr
	ethXMac      net.HardwareAddr

	vnetNSName           string
	vnetNSFileDescriptor int

	mode           string
	vlanID         int
	netnsClient    NetnsInterface
	netlink        netlink.NetlinkInterface
	netioshim      netio.NetIOInterface
	plClient       platform.ExecClient
	netUtilsClient networkutils.NetworkUtils
}

// Adds interfaces to the vnet (created if not existing) and vm namespace
func (client *NativeEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	// VM Namespace
	err := client.PopulateVM(epInfo)
	if err != nil {
		return err
	}
	// VNET Namespace
	err = ExecuteInNS(client.vnetNSName, epInfo, client.PopulateVnet)
	if err != nil {
		return err
	}

	return nil
}

// Called from AddEndpoints, Namespace: VM
func (client *NativeEndpointClient) PopulateVM(epInfo *EndpointInfo) error {
	vmNS, err := client.netnsClient.Get()
	if err != nil {
		return errors.Wrap(err, "failed to get vm ns handle")
	}
	// Inform current namespace
	returnedTo, err := GetCurrentThreadNamespace()
	if err != nil {
		return errors.Wrap(err, "failed to get vm ns")
	} else {
		log.Printf("[native] VM Namespace: %s", returnedTo.file.Name())
	}

	log.Printf("[native] Checking if NS exists...")
	vnetNS, existingErr := client.netnsClient.GetFromName(client.vnetNSName)
	// If the ns does not exist, the below code will trigger to create it

	if existingErr != nil {
		if !strings.Contains(strings.ToLower(existingErr.Error()), "no such file or directory") {
			return errors.Wrap(existingErr, "error other than vnet ns doesn't exist")
		} else {
			log.Printf("[native] No existing NS detected. Creating the vnet namespace and switching to it")
			vnetNS, err = client.netnsClient.NewNamed(client.vnetNSName)
			if err != nil {
				return errors.Wrap(err, "failed to create vnet ns")
			}

		}
	} else {
		log.Printf("[native] Existing NS detected.")
	}
	client.vnetNSFileDescriptor = vnetNS

	err = client.netnsClient.Set(vmNS)
	if err != nil {
		return errors.Wrap(err, "failed to set current ns to vm")
	}

	log.Printf("[native] Create the host vlan link after getting eth0: %s", client.eth0VethName)
	linkAttrs := vishnetlink.NewLinkAttrs()
	linkAttrs.Name = client.ethXVethName
	// Get parent interface index. Index is consistent across libraries.
	eth0, err := client.netioshim.GetNetworkInterfaceByName(client.eth0VethName)
	if err != nil {
		return errors.Wrap(err, "failed to get eth0 interface")
	}
	// Set the peer
	linkAttrs.ParentIndex = eth0.Index
	link := &vishnetlink.Vlan{
		LinkAttrs: linkAttrs,
		VlanId:    client.vlanID,
	}
	log.Printf("[native] Attempting to create eth0.X link in VM NS")
	// Attempting to create ethX
	existingErr = vishnetlink.LinkAdd(link)
	ethXCreated := true
	if existingErr != nil {
		if !strings.Contains(strings.ToLower(existingErr.Error()), "file exists") {
			return errors.Wrap(err, "error other than eth0.x already exists")
		} else {
			log.Printf("[native] eth0.X already exists")
			ethXCreated = false
		}
	}
	if ethXCreated {
		log.Printf("[native] Move vlan link (eth0.X) to vnet NS: %d", uintptr(client.vnetNSFileDescriptor))
		if err = client.netlink.SetLinkNetNs(client.ethXVethName, uintptr(client.vnetNSFileDescriptor)); err != nil {
			if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
				log.Errorf("deleting ethx veth failed on addendpoint failure:%v", delErr)
			}
			return errors.Wrap(err, "deleting ethx veth in vm ns due to addendpoint failure")
		}
	}

	if err = client.netUtilsClient.CreateEndpoint(client.vnetVethName, client.containerVethName); err != nil {
		return errors.Wrap(err, "failed to create veth pair")
	}

	if err = client.netlink.SetLinkNetNs(client.vnetVethName, uintptr(client.vnetNSFileDescriptor)); err != nil {
		if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
			log.Errorf("Deleting vnet veth failed on addendpoint failure:%v", delErr)
		}
		return errors.Wrap(err, "failed to move vnetVethName into vnet ns, deleting")
	}

	containerIf, err := client.netioshim.GetNetworkInterfaceByName(client.containerVethName)
	if err != nil {
		return errors.Wrap(err, "container veth does not exist")
	}
	client.containerMac = containerIf.HardwareAddr
	return nil
}

// Called from AddEndpoints, Namespace: Vnet
func (client *NativeEndpointClient) PopulateVnet(epInfo *EndpointInfo) error {
	ethXVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.ethXVethName)
	if err != nil {
		return errors.Wrap(err, "eth0.x doesn't exist")
	}
	client.ethXMac = ethXVethIf.HardwareAddr

	vnetVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.vnetVethName)
	if err != nil {
		return errors.Wrap(err, "vnet veth doesn't exist")
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
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return errors.Wrap(err, "failed to move endpoint to container ns")
	}
	return nil
}
func (client *NativeEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return errors.Wrap(err, "failed to setup container interface")
	}
	client.containerVethName = epInfo.IfName

	return nil
}

// Adds routes, arp entries, etc. to the vnet and container namespaces
func (client *NativeEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	// Container NS
	err := client.ConfigureContainerInterfacesAndRoutesImpl(epInfo)
	if err != nil {
		return err
	}

	// Switch to vnet NS and call ConfigureVnetInterfacesAndRoutes
	err = ExecuteInNS(client.vnetNSName, epInfo, client.ConfigureVnetInterfacesAndRoutesImpl)
	return err
}

// Called from ConfigureContainerInterfacesAndRoutes, Namespace: Container
func (client *NativeEndpointClient) ConfigureContainerInterfacesAndRoutesImpl(epInfo *EndpointInfo) error {

	if err := client.netUtilsClient.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return errors.Wrap(err, "failed to assign ips to container veth interface")
	}
	// kernel subnet route auto added by above call must be removed
	for _, ipAddr := range epInfo.IPAddresses {
		_, ipnet, _ := net.ParseCIDR(ipAddr.String())
		routeInfo := RouteInfo{
			Dst:      *ipnet,
			Scope:    netlink.RT_SCOPE_LINK,
			Protocol: netlink.RTPROT_KERNEL,
		}
		if err := deleteRoutes(client.netlink, client.netioshim, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
			return errors.Wrap(err, "failed to remove kernel subnet route")
		}
	}

	if err := client.AddDefaultRoutes(client.containerVethName); err != nil {
		return errors.Wrap(err, "failed Container ns add default routes")
	}
	if err := client.AddDefaultArp(client.containerVethName, client.vnetMac.String()); err != nil {
		return errors.Wrap(err, "failed Container ns add default arp")
	}
	return nil
}

// Called from ConfigureContainerInterfacesAndRoutes, Namespace: Vnet
func (client *NativeEndpointClient) ConfigureVnetInterfacesAndRoutesImpl(epInfo *EndpointInfo) error {

	err := client.netlink.SetLinkState(loopbackIf, true)
	if err != nil {
		return errors.Wrap(err, "failed to set loopback link state to up")
	}

	// Add route specifying which device the pod ip(s) are on
	routeInfoList := client.GetVnetRoutes(epInfo.IPAddresses)

	if err = client.AddDefaultRoutes(client.ethXVethName); err != nil {
		return errors.Wrap(err, "failed vnet ns add default/gateway routes (indempotent)")
	}
	if err = client.AddDefaultArp(client.ethXVethName, azureMac); err != nil {
		return errors.Wrap(err, "failed vnet ns add default arp entry (idempotent)")
	}
	if err := addRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return errors.Wrap(err, "failed adding routes to vnet specific to this container")
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
	// Add route for virtualgwip (ip route add 169.254.1.1/32 dev eth0)
	virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
	routeInfo := RouteInfo{
		Dst:   *virtualGwNet,
		Scope: netlink.RT_SCOPE_LINK,
	}
	// Difference between interface name in addRoutes and DevName: in RouteInfo?
	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}

	// Add default route (ip route add default via 169.254.1.1 dev eth0)
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
// gateway (169.254.1.1) to destMac on a particular interfaceName
// Example: (169.254.1.1) at 12:34:56:78:9a:bc [ether] PERM on <interfaceName>
func (client *NativeEndpointClient) AddDefaultArp(interfaceName string, destMac string) error {
	_, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
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
	routeInfoList := client.GetVnetRoutes(ep.IPAddresses)
	if err := deleteRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return errors.Wrap(err, "failed to remove routes")
	}

	routes, err := vishnetlink.RouteList(nil, vishnetlink.FAMILY_V4)
	if err != nil {
		return errors.Wrap(err, ("failed to get route list"))
	}
	log.Printf("[native] There are %d routes remaining: %v", len(routes), routes)
	if len(routes) <= numDefaultRoutes {
		// Deletes default arp, default routes, ethX; there are two default routes
		// so when we have <= numDefaultRoutes routes left, no containers use this namespace
		log.Printf("[native] Deleting namespace %s as no containers occupy it", client.vnetNSName)
		delErr := client.netnsClient.DeleteNamed(client.vnetNSName)
		if delErr != nil {
			return errors.Wrap(err, "failed to delete namespace")
		}
	}
	return nil
}

// Helper function that allows executing a function with one parameter in a VM namespace
// Does not work for process namespaces
func ExecuteInNS[T any](nsName string, param *T, f func(param *T) error) error {
	// Current namespace
	returnedTo, err := GetCurrentThreadNamespace()
	if err != nil {
		log.Errorf("[ExecuteInNS] Could not get NS we are in: %v", err)
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
			log.Errorf("[ExecuteInNS] Could not exit ns, err:%v.", err)
		}
		returnedTo, err := GetCurrentThreadNamespace()
		if err != nil {
			log.Errorf("[ExecuteInNS] Could not get NS we returned to: %v", err)
		} else {
			log.Printf("[ExecuteInNS] Returned to NS: %s", returnedTo.file.Name())
		}
	}()
	return f(param)
}
