//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/netns"
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
		// Populating VM with data and creating interfaces/links
		{
			name: "Add endpoints no existing vnet ns",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(2, "no such file or directory"),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints with existing vnet ns",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
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
				netnsClient:       netns.NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Failed to move vnetVethName into vnet NS, deleting : " + netlink.ErrorMockNetlink.Error() + " : netlink fail",
		},
		{
			name: "Add endpoints get interface fail for primary interface (eth0)",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Failed to get eth0 interface : " + netio.ErrMockNetIOFail.Error() + ":eth0",
		},
		{
			name: "Add endpoints get interface fail for getting container veth",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Container veth does not exist : " + netio.ErrMockNetIOFail.Error() + ":B1veth0",
		},
		{
			name: "Add endpoints NetNS Get fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(1, "netns failure"),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Failed to get VM NS handle : " + netns.ErrorMockNetns.Error() + " : netns failure",
		},
		{
			name: "Add endpoints NetNS GetFromName fail (with error other than file does not exists)",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(2, "netns failure"),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Error other than vnet NS doesn't exist : " + netns.ErrorMockNetns.Error() + " : netns failure",
		},
		{
			name: "Add endpoints NetNS Set fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(3, "netns failure"),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Failed to set current NS to VM : " + netns.ErrorMockNetns.Error() + " : netns failure",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.PopulateVM(tt.epInfo)
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
		// Populate the client with information from the vnet and set up vnet
		{
			name: "Add endpoints second half",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints fail check eth0.X exists",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : eth0.X doesn't exist : " + netio.ErrMockNetIOFail.Error() + ":eth0.1",
		},
		{
			name: "Add endpoints fail check vnet veth exists",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : vnet veth doesn't exist : " + netio.ErrMockNetIOFail.Error() + ":A1veth0",
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
		name       string
		client     *NativeEndpointClient
		ep         *endpoint
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Delete endpoint good path",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(0, ""),
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
			wantErr: false,
		},
		// You must have <= 2 ip routes on your machine for this to pass
		{
			name: "Delete endpoint fail to delete namespace",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient:       netns.NewMockNetns(5, "netns failure"),
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
			wantErr:    true,
			wantErrMsg: "NativeEndpointClient Error : Failed to delete namespace : " + netns.ErrorMockNetns.Error() + " : netns failure",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.DeleteEndpointsImpl(tt.ep)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, tt.wantErrMsg, err.Error(), "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
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
			name: "Configure interface and routes good path for container",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       netns.NewMockNetns(0, ""),
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
			name: "Configure interface and routes multiple IPs",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       netns.NewMockNetns(0, ""),
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
			name: "Configure interface and routes assign ip fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       netns.NewMockNetns(0, ""),
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
			name: "Configure interface and routes container 2nd default route added fail",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       netns.NewMockNetns(0, ""),
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
			wantErrMsg: "NativeEndpointClient Error : Failed Container NS add default routes : addRoutes failed: " + netio.ErrMockNetIOFail.Error() + ":B1veth0",
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
			name: "Configure interface and routes good path for vnet",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       netns.NewMockNetns(0, ""),
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
			// fail route that tells which device container ip is on for vnet
			name: "Configure interface and routes fail final routes for vnet",
			client: &NativeEndpointClient{
				eth0VethName:      "eth0",
				ethXVethName:      "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netnsClient:       netns.NewMockNetns(0, ""),
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
			wantErrMsg: "NativeEndpointClient Error : Failed adding routes to vnet specific to this container : addRoutes failed: " + netio.ErrMockNetIOFail.Error() + ":A1veth0",
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
