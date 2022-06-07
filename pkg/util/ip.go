package util

import (
	"fmt"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"inet.af/netaddr"
)

// GenerateAddressPoolStatus will generate a IP address for the node
func GenerateAddressPoolStatus(pool *bmaasv1alpha1.AddressPool) (poolStatus *bmaasv1alpha1.AddressStatus, err error) {
	poolStatus = pool.Status.DeepCopy()
	ipPrefix, err := netaddr.ParseIPPrefix(pool.Spec.CIDR)
	if err != nil {
		return nil, err
	}

	ipRange := ipPrefix.Range()
	len, err := availableAddresses(ipRange, pool.Spec.Gateway)
	if err != nil {
		return nil, err
	}
	poolStatus.StartAddress = ipRange.From().String()
	poolStatus.LastAddress = ipRange.To().String()
	poolStatus.AvailableAddresses = len
	poolStatus.Netmask = ipPrefix.IPNet().Mask.String()
	poolStatus.Status = bmaasv1alpha1.PoolReady
	poolStatus.AddressAllocation = make(map[string]bmaasv1alpha1.ObjectReferenceWithKind)
	return poolStatus, nil
}

// availableAddresses finds all address available in ip range, and excludes gateway if needed
func availableAddresses(ipRange netaddr.IPRange, gateway string) (len int, err error) {
	gw, err := netaddr.ParseIP(gateway)
	if err != nil {
		return len, err
	}
	for ip := ipRange.From(); ipRange.Contains(ip); ip = ip.Next() {
		if ip == gw {
			continue
		}
		len++
	}

	return len, nil
}

// AllocateAddress will allocate a custom Address or a dynamic address if address string is empty
func AllocateAddress(poolStatus *bmaasv1alpha1.AddressStatus, address string) (string, error) {

	if len(poolStatus.AddressAllocation) != 0 {
		node, ok := poolStatus.AddressAllocation[address]
		if ok {
			return "", fmt.Errorf("requested address %s is already allocated to node %s", address, node)
		}
	}

	ipRange, err := netaddr.ParseIPRange(fmt.Sprintf("%s-%s", poolStatus.StartAddress, poolStatus.LastAddress))
	if err != nil {
		return "", err
	}
	for ip := ipRange.From(); ipRange.Contains(ip); ip = ip.Next() {
		if len(poolStatus.AddressAllocation) != 0 {
			_, ok := poolStatus.AddressAllocation[ip.String()]
			if ok {
				continue
			}
		}
		// found an IP
		return ip.String(), nil

	}

	return "", fmt.Errorf("could not allocate an address as pool is already exhausted")
}

// DeallocateAddress will free up the address
func DeallocateAddress(poolStatus *bmaasv1alpha1.AddressStatus, address string) error {
	if _, ok := poolStatus.AddressAllocation[address]; !ok {
		return fmt.Errorf("address %s not allocated in the pool", address)
	}

	delete(poolStatus.AddressAllocation, address)
	return nil
}
