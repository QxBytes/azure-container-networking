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

var errorNativeEndpointClient = errors.New("TransparentEndpointClient Error")

func newErrorNativeEndpointClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorTransparentEndpointClient, errStr)
}

type NativeEndpointClient struct {
	hostPrimaryIfName string //So like eth0
	hostVethName      string //So like eth0.X
	vnetVethName      string //Peer is containerVethName
	containerVethName string //Peer is vnetVethName

	hostPrimaryMac net.HardwareAddr
	vnetMac        net.HardwareAddr
	containerMac   net.HardwareAddr
	hostVethMac    net.HardwareAddr

	vnetNSName string
	vnetNS     *Namespace
	dns        *DNSInfo

	mode           string
	vlanID         int
	netlink        netlink.NetlinkInterface
	netioshim      netio.NetIOInterface
	plClient       platform.ExecClient
	netUtilsClient networkutils.NetworkUtils
}

func NewNativeEndpointClient(
	extIf *externalInterface,
	hostVethName string,
	vnetVethName string,
	containerVethName string,
	vnetNSName string,
	mode string,
	vlanid int,
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
) *NativeEndpointClient {

	client := &NativeEndpointClient{
		hostPrimaryIfName: extIf.Name,
		hostVethName:      hostVethName,
		vnetVethName:      vnetVethName,
		containerVethName: containerVethName,
		hostPrimaryMac:    extIf.MacAddress,
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
	//Create the vnet namespace
	if _, err = netns.NewNamed(client.vnetNSName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	//Create said namespace (doesn't enter or exit yet)
	vnetNS, err := OpenNamespace(client.vnetNSName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.vnetNS = vnetNS

	//Create the host vlan link and move it to vnet NS
	linkAttrs := vishnetlink.NewLinkAttrs()
	linkAttrs.Name = client.hostVethName
	//TODO: linkAttrs.ParentIndex = eth0 (Primary)
	link := &vishnetlink.Vlan{
		LinkAttrs: linkAttrs,
		VlanId:    client.vlanID,
	}
	vishnetlink.LinkAdd(link)
	if err = client.netlink.SetLinkNetNs(client.hostVethName, client.vnetNS.GetFd()); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	//Create veth pair (automatically set to UP)
	if err = client.netUtilsClient.CreateEndpoint(client.vnetVethName, client.containerVethName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	//Move vnetVethName into vnet namespace (peer will be moved in MoveEndpointsToContainerNS)
	if err = client.netlink.SetLinkNetNs(client.vnetVethName, client.vnetNS.GetFd()); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	//If there is a failure, delete the links
	defer func() {
		if err != nil {
			//Delete vnet <> container
			if delErr := client.netlink.DeleteLink(client.hostVethName); delErr != nil {
				log.Errorf("Deleting veth failed on addendpoint failure:%v", delErr)
			}
			//Delete eth0 <> eth0.X

		}
	}()
	//Check that container veth exists. DOES it matter if they are in a different NS?
	containerIf, err := client.netioshim.GetNetworkInterfaceByName(client.containerVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.containerMac = containerIf.HardwareAddr

	//Check that host veth exists (eth0.X)
	hostVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.hostVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.hostVethMac = hostVethIf.HardwareAddr

	//Check that vnet veth exists
	vnetVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.hostVethName)
	if err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.vnetMac = vnetVethIf.HardwareAddr
	//Set MTU?

	return nil
}
func (client *NativeEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	//Described as rules on ip addresses on the container interface,

	// ip route add <podip> dev <hostveth>
	// This route is needed for incoming packets to pod to route via hostveth
	//Transparent has routes going to host, but we don't need that here, right?
	//Each endpoint only has two routes, which are the default.

	//What is arp proxy? Set up arp rules here?
	//AddEndpointRules vs. ConfigureContainerInterfacesAndRoutes
	return nil
}
func (client *NativeEndpointClient) DeleteEndpointRules(ep *endpoint) {
	return nil
}
func (client *NativeEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	return nil
}
func (client *NativeEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}
	client.containerVethName = epInfo.IfName

	return nil
}
func (client *NativeEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return newErrorNativeEndpointClient(err.Error())
	}

	// add route for virtualgwip
	// ip route add 169.254.1.1/32 dev eth0, but where do you specify eth0?
	// addRoutes says it adds route to containerVethName, so this appears to be a route to the container?

	virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
	routeInfo := RouteInfo{
		Dst:   *virtualGwNet,
		Scope: netlink.RT_SCOPE_LINK,
	}
	if err := addRoutes(client.netlink, client.netioshim, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
		return newErrorTransparentEndpointClient(err.Error())
	}

	// ip route add default via 169.254.1.1 dev eth0
	_, defaultIPNet, _ := net.ParseCIDR(defaultGwCidr)
	dstIP := net.IPNet{IP: net.ParseIP(defaultGw), Mask: defaultIPNet.Mask}
	routeInfo = RouteInfo{
		Dst: dstIP,
		Gw:  virtualGwIP,
	}
	if err := addRoutes(client.netlink, client.netioshim, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
		return err
	}

	// arp -s 169.254.1.1 e3:45:f4:ac:34:12 - add static arp entry for virtualgwip to hostveth interface mac
	log.Printf("[net] Adding static arp for IP address %v and MAC %v in Container namespace",
		virtualGwNet.String(), client.hostVethMac)
	if err := client.netlink.AddOrRemoveStaticArp(netlink.ADD,
		client.containerVethName,
		virtualGwNet.IP,
		client.hostVethMac,
		false); err != nil {
		return fmt.Errorf("Adding arp in container failed: %w", err)
	}

	return nil
}
func (client *NativeEndpointClient) DeleteEndpoints(ep *endpoint) error {
	return nil
}
