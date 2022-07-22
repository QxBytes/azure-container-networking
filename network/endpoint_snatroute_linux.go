package network

import (
	"fmt"

	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/Azure/azure-container-networking/platform"
)

func GetSnatHostIfName(epInfo *EndpointInfo) string {
	return fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])

}
func GetSnatContIfName(epInfo *EndpointInfo) string {
	return fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])
}
func AddSnatEndpoint(snatClient *snat.SnatClient) error {
	return snatClient.CreateSnatEndpoint()
}

func AddSnatEndpointRules(snatClient *snat.SnatClient, hostToNC, NCToHost bool, nl netlink.NetlinkInterface, plc platform.ExecClient) error {
	// Allow specific Private IPs via Snat Bridge
	if err := snatClient.AllowIPAddressesOnSnatBridge(); err != nil {
		return err
	}

	// Block Private IPs via Snat Bridge
	if err := snatClient.BlockIPAddressesOnSnatBridge(); err != nil {
		return err
	}
	nuc := networkutils.NewNetworkUtils(nl, plc)
	if err := nuc.EnableIPForwarding(snat.SnatBridgeName); err != nil {
		return err
	}

	if hostToNC {
		if err := snatClient.AllowInboundFromHostToNC(); err != nil {
			return err
		}
	}

	if NCToHost {
		return snatClient.AllowInboundFromNCToHost()
	}
	return nil
}

func MoveSnatEndpointToContainerNS(snatClient *snat.SnatClient, netnsPath string, nsID uintptr) error {
	return snatClient.MoveSnatEndpointToContainerNS(netnsPath, nsID)
}

func SetupSnatContainerInterface(snatClient *snat.SnatClient) error {
	return snatClient.SetupSnatContainerInterface()
}

func ConfigureSnatContainerInterface(snatClient *snat.SnatClient) error {
	return snatClient.ConfigureSnatContainerInterface()
}

func DeleteSnatEndpoint(snatClient *snat.SnatClient) error {
	return snatClient.DeleteSnatEndpoint()
}

func DeleteSnatEndpointRules(snatClient *snat.SnatClient, hostToNC, NCToHost bool) {
	if hostToNC {
		snatClient.DeleteInboundFromHostToNC()
	}

	if NCToHost {
		snatClient.DeleteInboundFromNCToHost()
	}
}
