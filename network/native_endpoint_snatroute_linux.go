package network

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/snat"
)

func (client *NativeEndpointClient) isSnatEnabled() bool {
	return client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns
}
func (client *NativeEndpointClient) NewSnatClient(snatBridgeIP string, localIP string, epInfo *EndpointInfo) {
	log.Printf("[native snat] %t %t %t %t", client.enableSnatOnHost, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.enableSnatForDns)
	if client.isSnatEnabled() {
		client.snatClient = snat.NewSnatClient(
			GetSnatHostIfName(epInfo),
			GetSnatContIfName(epInfo),
			localIP,
			snatBridgeIP,
			client.hostPrimaryMac.String(),
			epInfo.DNS.Servers,
			client.netlink,
			client.plClient,
		)
	}
}
func (client *NativeEndpointClient) AddSnatEndpoint() error {
	if client.isSnatEnabled() {
		if err := AddSnatEndpoint(client.snatClient); err != nil {
			return err
		}
	}
	return nil
}

func (client *NativeEndpointClient) AddSnatEndpointRules() error {
	if client.isSnatEnabled() {
		// Add route for 169.254.169.54 in host via azure0, otherwise it will route via snat bridge
		if err := AddSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.netlink, client.plClient); err != nil {
			return err
		}
	}

	return nil
}

func (client *NativeEndpointClient) MoveSnatEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	if client.isSnatEnabled() {
		return MoveSnatEndpointToContainerNS(client.snatClient, netnsPath, nsID)
	}

	return nil
}

func (client *NativeEndpointClient) SetupSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return SetupSnatContainerInterface(client.snatClient)
	}

	return nil
}

func (client *NativeEndpointClient) ConfigureSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return ConfigureSnatContainerInterface(client.snatClient)
	}

	return nil
}

func (client *NativeEndpointClient) DeleteSnatEndpoint() error {
	if client.isSnatEnabled() {
		return DeleteSnatEndpoint(client.snatClient)
	}

	return nil
}

func (client *NativeEndpointClient) DeleteSnatEndpointRules() {
	DeleteSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost)
}
