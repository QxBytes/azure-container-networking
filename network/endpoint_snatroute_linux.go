package network

import (
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
)

func GetSnatHostIfName(epInfo *EndpointInfo) string {
	return fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])
}

func GetSnatContIfName(epInfo *EndpointInfo) string {
	return fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])
}

func AddSnatEndpoint(snatClient *snat.Client) error {
	return errors.Wrap(snatClient.CreateSnatEndpoint(), "failed to add snat endpoint")
}

func AddSnatEndpointRules(snatClient *snat.Client, hostToNC, ncToHost bool, nl netlink.NetlinkInterface, plc platform.ExecClient) error {
	// Allow specific Private IPs via Snat Bridge
	if err := snatClient.AllowIPAddressesOnSnatBridge(); err != nil {
		return errors.Wrap(err, "failed to allow ip addresses on snat bridge")
	}

	// Block Private IPs via Snat Bridge
	if err := snatClient.BlockIPAddressesOnSnatBridge(); err != nil {
		return errors.Wrap(err, "failed to block ip addresses on snat bridge")
	}
	nuc := networkutils.NewNetworkUtils(nl, plc)
	if err := nuc.EnableIPForwarding(snat.SnatBridgeName); err != nil {
		return errors.Wrap(err, "failed to enable ip forwarding")
	}

	if hostToNC {
		if err := snatClient.AllowInboundFromHostToNC(); err != nil {
			return errors.Wrap(err, "failed to allow inbound from host to nc")
		}
	}

	if ncToHost {
		return errors.Wrap(snatClient.AllowInboundFromNCToHost(), "failed to allow inbound from nc to host")
	}
	return nil
}

func MoveSnatEndpointToContainerNS(snatClient *snat.Client, netnsPath string, nsID uintptr) error {
	return errors.Wrap(snatClient.MoveSnatEndpointToContainerNS(netnsPath, nsID), "failed to move snat endpoint to container ns")
}

func SetupSnatContainerInterface(snatClient *snat.Client) error {
	return errors.Wrap(snatClient.SetupSnatContainerInterface(), "failed to setup snat container interface")
}

func ConfigureSnatContainerInterface(snatClient *snat.Client) error {
	return errors.Wrap(snatClient.ConfigureSnatContainerInterface(), "failed to configure snat container interface")
}

func DeleteSnatEndpoint(snatClient *snat.Client) error {
	return errors.Wrap(snatClient.DeleteSnatEndpoint(), "failed to delete snat endpoint")
}

func DeleteSnatEndpointRules(snatClient *snat.Client, hostToNC, ncToHost bool) {
	if hostToNC {
		err := snatClient.DeleteInboundFromHostToNC()
		if err != nil {
			log.Errorf("failed to delete inbound from host to nc rules")
		}
	}

	if ncToHost {
		err := snatClient.DeleteInboundFromNCToHost()
		if err != nil {
			log.Errorf("failed to delete inbound from nc to host rules")
		}
	}
}
