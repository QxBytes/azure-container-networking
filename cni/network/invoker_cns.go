package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	cnscli "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/fsnotify"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/networkutils"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	expectedNumInterfacesWithDefaultRoutes = 1
)

var (
	errEmptyCNIArgs          = errors.New("empty CNI cmd args not allowed")
	errInvalidArgs           = errors.New("invalid arg(s)")
	errInvalidDefaultRouting = errors.New("add result requires exactly one interface with default routes")
	errInvalidGatewayIP      = errors.New("invalid gateway IP")
	overlayGatewayV6IP       = "fe80::1234:5678:9abc"
	watcherPath              = "/var/run/azure-vnet/deleteIDs"
)

type CNSIPAMInvoker struct {
	podName       string
	podNamespace  string
	cnsClient     cnsclient
	executionMode util.ExecutionMode
	ipamMode      util.IpamMode
}

type IPResultInfo struct {
	podIPAddress       string
	ncSubnetPrefix     uint8
	ncPrimaryIP        string
	ncGatewayIPAddress string
	hostSubnet         string
	hostPrimaryIP      string
	hostGateway        string
	nicType            cns.NICType
	macAddress         string
	skipDefaultRoutes  bool
	routes             []cns.Route
}

func (i IPResultInfo) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("podIPAddress", i.podIPAddress)
	encoder.AddUint8("ncSubnetPrefix", i.ncSubnetPrefix)
	encoder.AddString("ncPrimaryIP", i.ncPrimaryIP)
	encoder.AddString("ncGatewayIPAddress", i.ncGatewayIPAddress)
	encoder.AddString("hostSubnet", i.hostSubnet)
	encoder.AddString("hostPrimaryIP", i.hostPrimaryIP)
	encoder.AddString("hostGateway", i.hostGateway)
	encoder.AddString("nicType", string(i.nicType))
	encoder.AddString("macAddress", i.macAddress)
	encoder.AddBool("skipDefaultRoutes", i.skipDefaultRoutes)
	encoder.AddString("routes", fmt.Sprintf("%+v", i.routes))
	return nil
}

func NewCNSInvoker(podName, namespace string, cnsClient cnsclient, executionMode util.ExecutionMode, ipamMode util.IpamMode) *CNSIPAMInvoker {
	return &CNSIPAMInvoker{
		podName:       podName,
		podNamespace:  namespace,
		cnsClient:     cnsClient,
		executionMode: executionMode,
		ipamMode:      ipamMode,
	}
}

// Add uses the requestipconfig API in cns, and returns ipv4 and a nil ipv6 as CNS doesn't support IPv6 yet
func (invoker *CNSIPAMInvoker) Add(addConfig IPAMAddConfig) (IPAMAddResult, error) {
	// Parse Pod arguments.
	podInfo := cns.KubernetesPodInfo{
		PodName:      invoker.podName,
		PodNamespace: invoker.podNamespace,
	}

	logger.Info(podInfo.PodName)
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return IPAMAddResult{}, errors.Wrap(err, "Failed to unmarshal orchestrator context during add: %w")
	}

	if addConfig.args == nil {
		return IPAMAddResult{}, errEmptyCNIArgs
	}

	ipconfigs := cns.IPConfigsRequest{
		OrchestratorContext: orchestratorContext,
		PodInterfaceID:      GetEndpointID(addConfig.args),
		InfraContainerID:    addConfig.args.ContainerID,
	}

	logger.Info("Requesting IP for pod using ipconfig",
		zap.Any("pod", podInfo),
		zap.Any("ipconfig", ipconfigs))
	response, err := invoker.cnsClient.RequestIPs(context.TODO(), ipconfigs)
	if err != nil {
		if cnscli.IsUnsupportedAPI(err) {
			// If RequestIPs is not supported by CNS, use RequestIPAddress API
			logger.Error("RequestIPs not supported by CNS. Invoking RequestIPAddress API",
				zap.Any("infracontainerid", ipconfigs.InfraContainerID))
			ipconfig := cns.IPConfigRequest{
				OrchestratorContext: orchestratorContext,
				PodInterfaceID:      GetEndpointID(addConfig.args),
				InfraContainerID:    addConfig.args.ContainerID,
			}

			res, errRequestIP := invoker.cnsClient.RequestIPAddress(context.TODO(), ipconfig)
			if errRequestIP != nil {
				// if the old API fails as well then we just return the error
				logger.Error("Failed to request IP address from CNS using RequestIPAddress",
					zap.Any("infracontainerid", ipconfig.InfraContainerID),
					zap.Error(errRequestIP))
				return IPAMAddResult{}, errors.Wrap(errRequestIP, "Failed to get IP address from CNS")
			}
			response = &cns.IPConfigsResponse{
				Response: res.Response,
				PodIPInfo: []cns.PodIpInfo{
					res.PodIpInfo,
				},
			}
		} else {
			logger.Info("Failed to get IP address from CNS",
				zap.Error(err),
				zap.Any("response", response))
			return IPAMAddResult{}, errors.Wrap(err, "Failed to get IP address from CNS")
		}
	}

	addResult := IPAMAddResult{}
	numInterfacesWithDefaultRoutes := 0

	for i := 0; i < len(response.PodIPInfo); i++ {
		info := IPResultInfo{
			podIPAddress:       response.PodIPInfo[i].PodIPConfig.IPAddress,
			ncSubnetPrefix:     response.PodIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength,
			ncPrimaryIP:        response.PodIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress,
			ncGatewayIPAddress: response.PodIPInfo[i].NetworkContainerPrimaryIPConfig.GatewayIPAddress,
			hostSubnet:         response.PodIPInfo[i].HostPrimaryIPInfo.Subnet,
			hostPrimaryIP:      response.PodIPInfo[i].HostPrimaryIPInfo.PrimaryIP,
			hostGateway:        response.PodIPInfo[i].HostPrimaryIPInfo.Gateway,
			nicType:            response.PodIPInfo[i].NICType,
			macAddress:         response.PodIPInfo[i].MacAddress,
			skipDefaultRoutes:  response.PodIPInfo[i].SkipDefaultRoutes,
			routes:             response.PodIPInfo[i].Routes,
		}

		logger.Info("Received info for pod",
			zap.Any("ipInfo", info),
			zap.Any("podInfo", podInfo))

		//nolint:exhaustive // ignore exhaustive types check
		switch info.nicType {
		case cns.DelegatedVMNIC:
			// only handling single v4 PodIPInfo for DelegatedVMNICs at the moment, will have to update once v6 gets added
			if !info.skipDefaultRoutes {
				numInterfacesWithDefaultRoutes++
			}

			if err := configureSecondaryAddResult(&info, &addResult, &response.PodIPInfo[i].PodIPConfig); err != nil {
				return IPAMAddResult{}, err
			}
		default:
			// only count dualstack interface once
			if addResult.defaultInterfaceInfo.IPConfigs == nil {
				addResult.defaultInterfaceInfo.IPConfigs = make([]*network.IPConfig, 0)
				if !info.skipDefaultRoutes {
					numInterfacesWithDefaultRoutes++
				}
			}

			overlayMode := (invoker.ipamMode == util.V4Overlay) || (invoker.ipamMode == util.DualStackOverlay) || (invoker.ipamMode == util.Overlay)
			if err := configureDefaultAddResult(&info, &addConfig, &addResult, overlayMode); err != nil {
				return IPAMAddResult{}, err
			}
		}
	}

	// Make sure default routes exist for 1 interface
	if numInterfacesWithDefaultRoutes != expectedNumInterfacesWithDefaultRoutes {
		return IPAMAddResult{}, errInvalidDefaultRouting
	}

	return addResult, nil
}

func setHostOptions(ncSubnetPrefix *net.IPNet, options map[string]interface{}, info *IPResultInfo) error {
	// get the host ip
	hostIP := net.ParseIP(info.hostPrimaryIP)
	if hostIP == nil {
		return fmt.Errorf("Host IP address %v from response is invalid", info.hostPrimaryIP)
	}

	// get host gateway
	hostGateway := net.ParseIP(info.hostGateway)
	if hostGateway == nil {
		return fmt.Errorf("Host Gateway %v from response is invalid", info.hostGateway)
	}

	// this route is needed when the vm on subnet A needs to send traffic to a pod in subnet B on a different vm
	options[network.RoutesKey] = []network.RouteInfo{
		{
			Dst: *ncSubnetPrefix,
			Gw:  hostGateway,
		},
	}

	azureDNSUDPMatch := fmt.Sprintf(" -m addrtype ! --dst-type local -s %s -d %s -p %s --dport %d", ncSubnetPrefix.String(), networkutils.AzureDNS, iptables.UDP, iptables.DNSPort)
	azureDNSTCPMatch := fmt.Sprintf(" -m addrtype ! --dst-type local -s %s -d %s -p %s --dport %d", ncSubnetPrefix.String(), networkutils.AzureDNS, iptables.TCP, iptables.DNSPort)
	azureIMDSMatch := fmt.Sprintf(" -m addrtype ! --dst-type local -s %s -d %s -p %s --dport %d", ncSubnetPrefix.String(), networkutils.AzureIMDS, iptables.TCP, iptables.HTTPPort)

	snatPrimaryIPJump := fmt.Sprintf("%s --to %s", iptables.Snat, info.ncPrimaryIP)
	// we need to snat IMDS traffic to node IP, this sets up snat '--to'
	snatHostIPJump := fmt.Sprintf("%s --to %s", iptables.Snat, info.hostPrimaryIP)

	var iptableCmds []iptables.IPTableEntry
	if !iptables.ChainExists(iptables.V4, iptables.Nat, iptables.Swift) {
		iptableCmds = append(iptableCmds, iptables.GetCreateChainCmd(iptables.V4, iptables.Nat, iptables.Swift))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Postrouting, "", iptables.Swift) {
		iptableCmds = append(iptableCmds, iptables.GetAppendIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Postrouting, "", iptables.Swift))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Swift, azureDNSUDPMatch, snatPrimaryIPJump) {
		iptableCmds = append(iptableCmds, iptables.GetInsertIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Swift, azureDNSUDPMatch, snatPrimaryIPJump))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Swift, azureDNSTCPMatch, snatPrimaryIPJump) {
		iptableCmds = append(iptableCmds, iptables.GetInsertIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Swift, azureDNSTCPMatch, snatPrimaryIPJump))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Swift, azureIMDSMatch, snatHostIPJump) {
		iptableCmds = append(iptableCmds, iptables.GetInsertIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Swift, azureIMDSMatch, snatHostIPJump))
	}

	options[network.IPTablesKey] = iptableCmds

	return nil
}

// Delete calls into the releaseipconfiguration API in CNS
func (invoker *CNSIPAMInvoker) Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, args *cniSkel.CmdArgs, _ map[string]interface{}) error { //nolint
	var connectionErr *cnscli.ConnectionFailureErr
	// Parse Pod arguments.
	podInfo := cns.KubernetesPodInfo{
		PodName:      invoker.podName,
		PodNamespace: invoker.podNamespace,
	}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return err
	}

	if args == nil {
		return errEmptyCNIArgs
	}

	ipConfigs := cns.IPConfigsRequest{
		OrchestratorContext: orchestratorContext,
		PodInterfaceID:      GetEndpointID(args),
		InfraContainerID:    args.ContainerID,
	}

	if address != nil {
		ipConfigs.DesiredIPAddresses = append(ipConfigs.DesiredIPAddresses, address.IP.String())
	} else {
		logger.Info("CNS invoker called with empty IP address")
	}

	if err := invoker.cnsClient.ReleaseIPs(context.TODO(), ipConfigs); err != nil {
		if cnscli.IsUnsupportedAPI(err) {
			// If ReleaseIPs is not supported by CNS, use ReleaseIPAddress API
			logger.Error("ReleaseIPs not supported by CNS. Invoking ReleaseIPAddress API",
				zap.Any("ipconfigs", ipConfigs))

			ipConfig := cns.IPConfigRequest{
				OrchestratorContext: orchestratorContext,
				PodInterfaceID:      GetEndpointID(args),
				InfraContainerID:    args.ContainerID,
			}

			if err = invoker.cnsClient.ReleaseIPAddress(context.TODO(), ipConfig); err != nil {
				if errors.As(err, &connectionErr) {
					addErr := fsnotify.AddFile(ipConfigs.PodInterfaceID, args.ContainerID, watcherPath)
					if addErr != nil {
						logger.Error("Failed to add file to watcher", zap.String("podInterfaceID", ipConfigs.PodInterfaceID), zap.String("containerID", args.ContainerID), zap.Error(addErr))
						return errors.Wrap(addErr, fmt.Sprintf("failed to add file to watcher with containerID %s and podInterfaceID %s", args.ContainerID, ipConfigs.PodInterfaceID))
					}
				} else {
					logger.Error("Failed to release IP address from CNS using ReleaseIPAddress ",
						zap.String("infracontainerid", ipConfigs.InfraContainerID),
						zap.Error(err))

					return errors.Wrap(err, fmt.Sprintf("failed to release IP %v using ReleaseIPAddress with err ", ipConfig.DesiredIPAddress)+"%w")
				}
			}
		} else {
			if errors.As(err, &connectionErr) {
				addErr := fsnotify.AddFile(ipConfigs.PodInterfaceID, args.ContainerID, watcherPath)
				if addErr != nil {
					logger.Error("Failed to add file to watcher", zap.String("podInterfaceID", ipConfigs.PodInterfaceID), zap.String("containerID", args.ContainerID), zap.Error(addErr))
					return errors.Wrap(addErr, fmt.Sprintf("failed to add file to watcher with containerID %s and podInterfaceID %s", args.ContainerID, ipConfigs.PodInterfaceID))
				}
			} else {
				logger.Error("Failed to release IP address",
					zap.String("infracontainerid", ipConfigs.InfraContainerID),
					zap.Error(err))
				return errors.Wrap(err, fmt.Sprintf("failed to release IP %v using ReleaseIPs with err ", ipConfigs.DesiredIPAddresses)+"%w")
			}
		}
	}

	return nil
}

func getRoutes(cnsRoutes []cns.Route, skipDefaultRoutes bool) ([]network.RouteInfo, error) {
	routes := make([]network.RouteInfo, 0)
	for _, route := range cnsRoutes {
		_, dst, routeErr := net.ParseCIDR(route.IPAddress)
		if routeErr != nil {
			return nil, fmt.Errorf("unable to parse destination %s: %w", route.IPAddress, routeErr)
		}

		gw := net.ParseIP(route.GatewayIPAddress)
		if gw == nil && skipDefaultRoutes {
			return nil, errors.Wrap(errInvalidGatewayIP, route.GatewayIPAddress)
		}

		routes = append(routes,
			network.RouteInfo{
				Dst: *dst,
				Gw:  gw,
			})
	}

	return routes, nil
}

func configureDefaultAddResult(info *IPResultInfo, addConfig *IPAMAddConfig, addResult *IPAMAddResult, overlayMode bool) error {
	// set the NC Primary IP in options
	// SNATIPKey is not set for ipv6
	if net.ParseIP(info.ncPrimaryIP).To4() != nil {
		addConfig.options[network.SNATIPKey] = info.ncPrimaryIP
	}

	ip, ncIPNet, err := net.ParseCIDR(info.podIPAddress + "/" + fmt.Sprint(info.ncSubnetPrefix))
	if ip == nil {
		return errors.Wrap(err, "Unable to parse IP from response: "+info.podIPAddress+" with err %w")
	}

	ncgw := net.ParseIP(info.ncGatewayIPAddress)
	if ncgw == nil {
		// TODO: Remove v4overlay and dualstackoverlay options, after 'overlay' rolls out in AKS-RP
		if !overlayMode {
			return errors.Wrap(errInvalidArgs, "%w: Gateway address "+info.ncGatewayIPAddress+" from response is invalid")
		}

		if net.ParseIP(info.podIPAddress).To4() != nil { //nolint:gocritic
			ncgw, err = getOverlayGateway(ncIPNet)
			if err != nil {
				return err
			}
		} else if net.ParseIP(info.podIPAddress).To16() != nil {
			ncgw = net.ParseIP(overlayGatewayV6IP)
		} else {
			return errors.Wrap(err, "No podIPAddress is found: %w")
		}
	}

	if ip := net.ParseIP(info.podIPAddress); ip != nil {
		defaultInterfaceInfo := &addResult.defaultInterfaceInfo
		defaultRouteDstPrefix := network.Ipv4DefaultRouteDstPrefix
		if ip.To4() == nil {
			defaultRouteDstPrefix = network.Ipv6DefaultRouteDstPrefix
			addResult.ipv6Enabled = true
		}

		defaultInterfaceInfo.IPConfigs = append(defaultInterfaceInfo.IPConfigs,
			&network.IPConfig{
				Address: net.IPNet{
					IP:   ip,
					Mask: ncIPNet.Mask,
				},
				Gateway: ncgw,
			})

		routes, getRoutesErr := getRoutes(info.routes, info.skipDefaultRoutes)
		if getRoutesErr != nil {
			return getRoutesErr
		}

		if len(routes) > 0 {
			defaultInterfaceInfo.Routes = append(defaultInterfaceInfo.Routes, routes...)
		} else { // add default routes if none are provided
			defaultInterfaceInfo.Routes = append(defaultInterfaceInfo.Routes, network.RouteInfo{
				Dst: defaultRouteDstPrefix,
				Gw:  ncgw,
			})
		}

		addResult.defaultInterfaceInfo.SkipDefaultRoutes = info.skipDefaultRoutes
	}

	// get the name of the primary IP address
	_, hostIPNet, err := net.ParseCIDR(info.hostSubnet)
	if err != nil {
		return fmt.Errorf("unable to parse hostSubnet: %w", err)
	}

	addResult.hostSubnetPrefix = *hostIPNet
	addResult.defaultInterfaceInfo.NICType = cns.InfraNIC

	// set subnet prefix for host vm
	// setHostOptions will execute if IPAM mode is not v4 overlay and not dualStackOverlay mode
	// TODO: Remove v4overlay and dualstackoverlay options, after 'overlay' rolls out in AKS-RP
	if !overlayMode {
		if err := setHostOptions(ncIPNet, addConfig.options, info); err != nil {
			return err
		}
	}

	return nil
}

func configureSecondaryAddResult(info *IPResultInfo, addResult *IPAMAddResult, podIPConfig *cns.IPSubnet) error {
	ip, ipnet, err := podIPConfig.GetIPNet()
	if ip == nil {
		return errors.Wrap(err, "Unable to parse IP from response: "+info.podIPAddress+" with err %w")
	}

	macAddress, err := net.ParseMAC(info.macAddress)
	if err != nil {
		return errors.Wrap(err, "Invalid mac address")
	}

	routes, err := getRoutes(info.routes, info.skipDefaultRoutes)
	if err != nil {
		return err
	}

	result := network.InterfaceInfo{
		IPConfigs: []*network.IPConfig{
			{
				Address: net.IPNet{
					IP:   ip,
					Mask: ipnet.Mask,
				},
			},
		},
		Routes:            routes,
		NICType:           info.nicType,
		MacAddress:        macAddress,
		SkipDefaultRoutes: info.skipDefaultRoutes,
	}

	addResult.secondaryInterfacesInfo = append(addResult.secondaryInterfacesInfo, result)

	return nil
}
