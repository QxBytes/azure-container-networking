package network

import (
	"fmt"

	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/snat"
)

func NativeNewSnatClient(client *NativeEndpointClient, snatBridgeIP string, localIP string, epInfo *EndpointInfo) {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		hostIfName := fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])
		contIfName := fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])

		client.snatClient = snat.NewSnatClient(hostIfName,
			contIfName,
			localIP,
			snatBridgeIP,
			client.hostPrimaryMac.String(),
			epInfo.DNS.Servers,
			client.netlink,
			nil,
			client.plClient,
		)
	}
}

func NativeAddSnatEndpoint(client *NativeEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		if err := client.snatClient.CreateSnatEndpoint(client.bridgeName); err != nil {
			return err
		}
	}

	return nil
}

func NativeAddSnatEndpointRules(client *NativeEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		// Allow specific Private IPs via Snat Bridge
		if err := client.snatClient.AllowIPAddressesOnSnatBridge(); err != nil {
			return err
		}

		// Block Private IPs via Snat Bridge
		if err := client.snatClient.BlockIPAddressesOnSnatBridge(); err != nil {
			return err
		}

		// Add route for 169.254.169.54 in host via azure0, otherwise it will route via snat bridge
		if err := AddStaticRoute(client.netlink, client.netioshim, snat.ImdsIP, client.bridgeName); err != nil {
			return err
		}

		nuc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
		if err := nuc.EnableIPForwarding(snat.SnatBridgeName); err != nil {
			return err
		}

		if client.allowInboundFromHostToNC {
			if err := client.snatClient.AllowInboundFromHostToNC(); err != nil {
				return err
			}
		}

		if client.allowInboundFromNCToHost {
			return client.snatClient.AllowInboundFromNCToHost()
		}
	}

	return nil
}

func NativeMoveSnatEndpointToContainerNS(client *NativeEndpointClient, netnsPath string, nsID uintptr) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.MoveSnatEndpointToContainerNS(netnsPath, nsID)
	}

	return nil
}

func NativeSetupSnatContainerInterface(client *NativeEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.SetupSnatContainerInterface()
	}

	return nil
}

func NativeConfigureSnatContainerInterface(client *NativeEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.ConfigureSnatContainerInterface()
	}

	return nil
}

func NativeDeleteSnatEndpoint(client *NativeEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.DeleteSnatEndpoint()
	}

	return nil
}

func NativeDeleteSnatEndpointRules(client *NativeEndpointClient) {
	if client.allowInboundFromHostToNC {
		client.snatClient.DeleteInboundFromHostToNC()
	}

	if client.allowInboundFromNCToHost {
		client.snatClient.DeleteInboundFromNCToHost()
	}
}
