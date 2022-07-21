package network

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/snat"
)

func nativeEnableSnat(client *NativeEndpointClient) bool {
	return client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns
}
func NativeNewSnatClient(client *NativeEndpointClient, snatBridgeIP string, localIP string, epInfo *EndpointInfo) {
	log.Printf("[native snat] %t %t %t %t", client.enableSnatOnHost, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.enableSnatForDns)
	if nativeEnableSnat(client) {
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
func NativeAddSnatEndpoint(client *NativeEndpointClient) error {
	if nativeEnableSnat(client) {
		if err := AddSnatEndpoint(client.snatClient); err != nil {
			return err
		}
	}
	return nil
}

func NativeAddSnatEndpointRules(client *NativeEndpointClient) error {
	if nativeEnableSnat(client) {
		// Add route for 169.254.169.54 in host via azure0, otherwise it will route via snat bridge
		if err := AddSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.netlink, client.plClient); err != nil {
			return err
		}
	}

	return nil
}

func NativeMoveSnatEndpointToContainerNS(client *NativeEndpointClient, netnsPath string, nsID uintptr) error {
	if nativeEnableSnat(client) {
		return MoveSnatEndpointToContainerNS(client.snatClient, netnsPath, nsID)
	}

	return nil
}

func NativeSetupSnatContainerInterface(client *NativeEndpointClient) error {
	if nativeEnableSnat(client) {
		return SetupSnatContainerInterface(client.snatClient)
	}

	return nil
}

func NativeConfigureSnatContainerInterface(client *NativeEndpointClient) error {
	if nativeEnableSnat(client) {
		return ConfigureSnatContainerInterface(client.snatClient)
	}

	return nil
}

func NativeDeleteSnatEndpoint(client *NativeEndpointClient) error {
	if nativeEnableSnat(client) {
		return DeleteSnatEndpoint(client.snatClient)
	}

	return nil
}

func NativeDeleteSnatEndpointRules(client *NativeEndpointClient) {
	DeleteSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost)
}
