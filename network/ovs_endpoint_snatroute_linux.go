package network

import (
	"github.com/Azure/azure-container-networking/network/snat"
)

func (client *OVSEndpointClient) isSnatEnabled() bool {
	return client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns
}
func (client *OVSEndpointClient) NewSnatClient(snatBridgeIP string, localIP string, epInfo *EndpointInfo) {
	if client.isSnatEnabled() {
		client.snatClient = snat.NewSnatClient(
			GetSnatHostIfName(epInfo),
			GetSnatContIfName(epInfo),
			localIP,
			snatBridgeIP,
			client.hostPrimaryMac,
			epInfo.DNS.Servers,
			client.netlink,
			client.plClient,
		)
	}
}
func (client *OVSEndpointClient) AddSnatEndpoint() error {
	if client.isSnatEnabled() {
		if err := AddSnatEndpoint(client.snatClient); err != nil {
			return err
		}
		if err := client.ovsctlClient.AddPortOnOVSBridge(snat.AzureSnatVeth1, client.bridgeName, 0); err != nil {
			return err
		}
	}
	return nil
}

func (client *OVSEndpointClient) AddSnatEndpointRules() error {
	if client.isSnatEnabled() {
		// Add route for 169.254.169.54 in host via azure0, otherwise it will route via snat bridge
		if err := AddSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.netlink, client.plClient); err != nil {
			return err
		}
		if err := AddStaticRoute(client.netlink, client.netioshim, snat.ImdsIP, client.bridgeName); err != nil {
			return err
		}
	}

	return nil
}

func (client *OVSEndpointClient) MoveSnatEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	if client.isSnatEnabled() {
		return MoveSnatEndpointToContainerNS(client.snatClient, netnsPath, nsID)
	}

	return nil
}

func (client *OVSEndpointClient) SetupSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return SetupSnatContainerInterface(client.snatClient)
	}

	return nil
}

func (client *OVSEndpointClient) ConfigureSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return ConfigureSnatContainerInterface(client.snatClient)
	}

	return nil
}

func (client *OVSEndpointClient) DeleteSnatEndpoint() error {
	if client.isSnatEnabled() {
		return DeleteSnatEndpoint(client.snatClient)
	}

	return nil
}

func (client *OVSEndpointClient) DeleteSnatEndpointRules() {
	DeleteSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost)
}
