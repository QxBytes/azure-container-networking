//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/require"
)

func TestNativeAddEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name       string
		client     *NativeEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add endpoints",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints netlink fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : " + netlink.ErrorMockNetlink.Error() + " : netlink fail",
		},
		{
			name: "Add endpoints get interface fail for primary interface",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":eth0",
		},
		{
			name: "Add endpoints get interface fail for getting container veth",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":B1veth0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.PopulateClient(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, tt.wantErrMsg, err.Error(), "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}

	tests = []struct {
		name       string
		client     *NativeEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add endpoints second half",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints netlink fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":eth0.1",
		},
		{
			name: "Add endpoints netlink fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":A1veth0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.PopulateVnet(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, tt.wantErrMsg, err.Error(), "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestNativeDeleteEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name   string
		client *NativeEndpointClient
		ep     *endpoint
	}{
		{
			name: "Delete endpoint rules good path",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			ep: &endpoint{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.client.DeleteEndpointsImpl(tt.ep)
		})
	}
}

func TestNativeConfigureContainerInterfacesAndRoutes(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	vnetMac, _ := net.ParseMAC("ab:cd:ef:12:34:56")

	tests := []struct {
		name       string
		client     *NativeEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Configure Interface and routes good path",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure Interface and routes multiple IPs",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
					{
						IP:   net.ParseIP("192.168.0.6"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
					{
						IP:   net.ParseIP("192.168.0.8"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure Interface and routes assign ip fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "netlink fail",
		},
		{
			name: "Configure Interface and routes container final route (2nd default route) fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 3),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : addRoutes failed: " + netio.ErrMockNetIOFail.Error() + ":B1veth0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ConfigureContainerInterfacesAndRoutesImpl(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
	tests = []struct {
		name       string
		client     *NativeEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Configure Interface and routes good path second half",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure Interface and routes final routes for vnet",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 3),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : addRoutes failed: " + netio.ErrMockNetIOFail.Error() + ":A1veth0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ConfigureVnetInterfacesAndRoutesImpl(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
