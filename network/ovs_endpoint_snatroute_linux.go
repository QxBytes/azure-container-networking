package network

import (
	"github.com/Azure/azure-container-networking/network/snat"
)

func ovsEnableSnat(client *OVSEndpointClient) bool {
	return client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns
}
func OVSNewSnatClient(client *OVSEndpointClient, snatBridgeIP string, localIP string, epInfo *EndpointInfo) {
	if ovsEnableSnat(client) {
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
func OVSAddSnatEndpoint(client *OVSEndpointClient) error {
	if ovsEnableSnat(client) {
		if err := AddSnatEndpoint(client.snatClient); err != nil {
			return err
		}
		if err := client.ovsctlClient.AddPortOnOVSBridge(snat.AzureSnatVeth1, client.bridgeName, 0); err != nil {
			return err
		}
	}
	return nil
}

func OVSAddSnatEndpointRules(client *OVSEndpointClient) error {
	if ovsEnableSnat(client) {
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

func OVSMoveSnatEndpointToContainerNS(client *OVSEndpointClient, netnsPath string, nsID uintptr) error {
	if ovsEnableSnat(client) {
		return MoveSnatEndpointToContainerNS(client.snatClient, netnsPath, nsID)
	}

	return nil
}

func OVSSetupSnatContainerInterface(client *OVSEndpointClient) error {
	if ovsEnableSnat(client) {
		return SetupSnatContainerInterface(client.snatClient)
	}

	return nil
}

func OVSConfigureSnatContainerInterface(client *OVSEndpointClient) error {
	if ovsEnableSnat(client) {
		return ConfigureSnatContainerInterface(client.snatClient)
	}

	return nil
}

func OVSDeleteSnatEndpoint(client *OVSEndpointClient) error {
	if ovsEnableSnat(client) {
		return DeleteSnatEndpoint(client.snatClient)
	}

	return nil
}

func OVSDeleteSnatEndpointRules(client *OVSEndpointClient) {
	DeleteSnatEndpointRules(client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost)
}
