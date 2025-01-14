// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
)

var (
	errAZsNotEqual = errors.New("public subnets and private subnets do not span the same availability zones")

	minAZs = 2
)

// Validate returns nil if Environment is configured correctly.
func (e Environment) Validate() error {
	if err := e.environmentConfig.Validate(); err != nil {
		return fmt.Errorf(`validate "network": %w`, err)
	}
	return nil
}

// Validate returns nil if environmentConfig is configured correctly.
func (e environmentConfig) Validate() error {
	if err := e.Network.Validate(); err != nil {
		return fmt.Errorf(`validate "network": %w`, err)
	}
	if err := e.Observability.Validate(); err != nil {
		return fmt.Errorf(`validate "observability": %w`, err)
	}
	if err := e.HTTPConfig.Validate(); err != nil {
		return fmt.Errorf(`validate "http config": %w`, err)
	}

	if e.HTTPConfig.Private.InternalALBSubnets != nil {
		if !e.Network.VPC.imported() {
			return errors.New("in order to specify internal ALB subnet placement, subnets must be imported")
		}
		if err := e.validateInternalALBSubnets(); err != nil {
			return err
		}
	}
	return nil
}

// Validate returns nil if environmentNetworkConfig is configured correctly.
func (n environmentNetworkConfig) Validate() error {
	if err := n.VPC.Validate(); err != nil {
		return fmt.Errorf(`validate "vpc": %w`, err)
	}
	return nil
}

// Validate returns nil if environmentVPCConfig is configured correctly.
func (cfg environmentVPCConfig) Validate() error {
	if cfg.imported() && cfg.managedVPCCustomized() {
		return errors.New(`cannot import VPC resources (with "id" fields) and customize VPC resources (with "cidr" and "az" fields) at the same time`)
	}
	if err := cfg.Subnets.Validate(); err != nil {
		return fmt.Errorf(`validate "subnets": %w`, err)
	}
	if cfg.imported() {
		if err := cfg.validateImportedVPC(); err != nil {
			return fmt.Errorf(`validate "subnets" for an imported VPC: %w`, err)
		}
	}
	if cfg.managedVPCCustomized() {
		if err := cfg.validateManagedVPC(); err != nil {
			return fmt.Errorf(`validate "subnets" for an adjusted VPC: %w`, err)
		}
	}
	return nil
}

func (cfg environmentVPCConfig) validateImportedVPC() error {
	for idx, subnet := range cfg.Subnets.Public {
		if aws.StringValue(subnet.SubnetID) == "" {
			return fmt.Errorf(`validate public[%d]: %w`, idx, &errFieldMustBeSpecified{
				missingField: "id",
			})
		}
	}
	for idx, subnet := range cfg.Subnets.Private {
		if aws.StringValue(subnet.SubnetID) == "" {
			return fmt.Errorf(`validate private[%d]: %w`, idx, &errFieldMustBeSpecified{
				missingField: "id",
			})
		}
	}
	switch {
	case len(cfg.Subnets.Private)+len(cfg.Subnets.Public) <= 0:
		return errors.New(`VPC must have subnets in order to proceed with environment creation`)
	case len(cfg.Subnets.Public) == 1:
		return errors.New(`validate "public": at least two public subnets must be imported to enable Load Balancing`)
	case len(cfg.Subnets.Private) == 1:
		return errors.New(`validate "private": at least two private subnets must be imported`)
	}
	return nil
}

func (cfg environmentVPCConfig) validateManagedVPC() error {
	var (
		publicAZs    = make(map[string]struct{})
		privateAZs   = make(map[string]struct{})
		publicCIDRs  = make(map[string]struct{})
		privateCIDRs = make(map[string]struct{})
	)
	var exists = struct{}{}
	for idx, subnet := range cfg.Subnets.Public {
		if aws.StringValue((*string)(subnet.CIDR)) == "" {
			return fmt.Errorf(`validate public[%d]: %w`, idx, &errFieldMustBeSpecified{
				missingField: "cidr",
			})
		}
		publicCIDRs[aws.StringValue((*string)(subnet.CIDR))] = exists
		if aws.StringValue(subnet.AZ) != "" {
			publicAZs[aws.StringValue(subnet.AZ)] = exists
		}
	}
	for idx, subnet := range cfg.Subnets.Private {
		if aws.StringValue((*string)(subnet.CIDR)) == "" {
			return fmt.Errorf(`validate private[%d]: %w`, idx, &errFieldMustBeSpecified{
				missingField: "cidr",
			})
		}
		privateCIDRs[aws.StringValue((*string)(subnet.CIDR))] = exists
		if aws.StringValue(subnet.AZ) != "" {
			privateAZs[aws.StringValue(subnet.AZ)] = exists
		}
	}
	// NOTE: the following are constraints on az:
	// 1. #az = 0, or #az = #public_subnets = #private_subnets.
	// 2. set(az_for_public) = set(az_for_private).
	// 3, If configured at all, the number of AZ must be >= 2.
	if !areSetsEqual(publicAZs, privateAZs) {
		return errAZsNotEqual
	}
	numAZs := len(publicAZs)
	if numAZs == 0 {
		return nil
	}
	if numAZs < minAZs {
		return fmt.Errorf(`require at least %d availability zones`, minAZs)
	}
	if len(publicCIDRs) != numAZs {
		return fmt.Errorf(`validate "public": number of public subnet CIDRs (%d) does not match number of AZs (%d)`, len(publicCIDRs), len(publicAZs))
	}
	if len(privateCIDRs) != numAZs {
		return fmt.Errorf(`validate "private": number of private subnet CIDRs (%d) does not match number of AZs (%d)`, len(privateCIDRs), len(publicAZs))
	}
	return nil
}

// Validate returns nil if subnetsConfiguration is configured correctly.
func (cs subnetsConfiguration) Validate() error {
	for idx, subnet := range cs.Public {
		if err := subnet.Validate(); err != nil {
			return fmt.Errorf(`validate "public[%d]": %w`, idx, err)
		}
	}
	for idx, subnet := range cs.Private {
		if err := subnet.Validate(); err != nil {
			return fmt.Errorf(`validate "private[%d]": %w`, idx, err)
		}
	}
	return nil
}

// Validate returns nil if subnetConfiguration is configured correctly.
func (c subnetConfiguration) Validate() error {
	if c.SubnetID != nil && c.CIDR != nil {
		return &errFieldMutualExclusive{
			firstField:  "id",
			secondField: "cidr",
			mustExist:   false,
		}
	}
	if c.SubnetID != nil && c.AZ != nil {
		return &errFieldMutualExclusive{
			firstField:  "id",
			secondField: "az",
			mustExist:   false,
		}
	}
	return nil
}

// Validate returns nil if environmentObservability is configured correctly.
func (o environmentObservability) Validate() error {
	return nil
}

// Validate returns nil if environmentHTTPConfig is configured correctly.
func (cfg environmentHTTPConfig) Validate() error {
	if err := cfg.Public.Validate(); err != nil {
		return fmt.Errorf(`validate "public": %w`, err)
	}
	if err := cfg.Private.Validate(); err != nil {
		return fmt.Errorf(`validate "private": %w`, err)
	}
	return nil
}

// Validate returns nil if publicHTTPConfig is configured correctly.
func (cfg publicHTTPConfig) Validate() error {
	for idx, certARN := range cfg.Certificates {
		if _, err := arn.Parse(certARN); err != nil {
			return fmt.Errorf(`parse "certificates[%d]": %w`, idx, err)
		}
	}
	return nil
}

// Validate returns nil if privateHTTPConfig is configured correctly.
func (cfg privateHTTPConfig) Validate() error {
	for idx, certARN := range cfg.Certificates {
		if _, err := arn.Parse(certARN); err != nil {
			return fmt.Errorf(`parse "certificates[%d]": %w`, idx, err)
		}
	}
	return nil
}

// Validate returns nil if environmentCDNConfig is configured correctly.
func (cfg environmentCDNConfig) Validate() error {
	if cfg.CDNConfig.IsEmpty() {
		return nil
	}
	return cfg.CDNConfig.Validate()
}

// Validate is a no-op for AdvancedCDNConfig.
func (cfg advancedCDNConfig) Validate() error {
	return nil
}

func (c environmentConfig) validateInternalALBSubnets() error {
	isImported := make(map[string]bool)
	for _, placementSubnet := range c.HTTPConfig.Private.InternalALBSubnets {
		for _, subnet := range append(c.Network.VPC.Subnets.Private, c.Network.VPC.Subnets.Public...) {
			if placementSubnet == aws.StringValue(subnet.SubnetID) {
				isImported[placementSubnet] = true
			}
		}
	}
	if len(isImported) != len(c.HTTPConfig.Private.InternalALBSubnets) {
		return fmt.Errorf("subnet(s) specified for internal ALB placement not imported")
	}
	return nil
}

func areSetsEqual[T comparable](a map[T]struct{}, b map[T]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
