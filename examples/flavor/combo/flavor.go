package main

import (
	"encoding/json"
	"errors"
	"github.com/docker/infrakit/pkg/plugin/group"
	"github.com/docker/infrakit/pkg/plugin/group/types"
	"github.com/docker/infrakit/pkg/spi/flavor"
	"github.com/docker/infrakit/pkg/spi/instance"
	"strings"
)

// Spec is the model of the plugin Properties.
type Spec struct {
	Flavors []types.FlavorPlugin
}

// NewPlugin creates a Flavor Combo plugin that chains multiple flavors in a sequence.  Each flavor
func NewPlugin(flavorPlugins group.FlavorPluginLookup) flavor.Plugin {
	return flavorCombo{flavorPlugins: flavorPlugins}
}

type flavorCombo struct {
	flavorPlugins group.FlavorPluginLookup
}

func (f flavorCombo) Validate(flavorProperties json.RawMessage, allocation types.AllocationMethod) error {
	s := Spec{}
	return json.Unmarshal(flavorProperties, &s)
}

func (f flavorCombo) Healthy(flavorProperties json.RawMessage, inst instance.Description) (flavor.Health, error) {
	// The overall health of the flavor combination is taken as the 'lowest common demoninator' of the configured
	// flavors.  Only flavor.Healthy is reported if all flavors report flavor.Healthy.  flavor.Unhealthy or
	// flavor.UnknownHealth is returned as soon as any Flavor reports that value.

	s := Spec{}
	if err := json.Unmarshal(flavorProperties, &s); err != nil {
		return flavor.Unknown, err
	}

	for _, pluginSpec := range s.Flavors {
		plugin, err := f.flavorPlugins(pluginSpec.Plugin)
		if err != nil {
			return flavor.Unknown, err
		}

		health, err := plugin.Healthy(types.RawMessage(pluginSpec.Properties), inst)
		if err != nil || health != flavor.Healthy {
			return health, err
		}
	}

	return flavor.Healthy, nil
}

func (f flavorCombo) Drain(flavorProperties json.RawMessage, inst instance.Description) error {
	// Draining is attempted on all flavors regardless of errors encountered.  All errors encountered are combined
	// and returned.

	s := Spec{}
	if err := json.Unmarshal(flavorProperties, &s); err != nil {
		return err
	}

	errs := []string{}

	for _, pluginSpec := range s.Flavors {
		plugin, err := f.flavorPlugins(pluginSpec.Plugin)
		if err != nil {
			errs = append(errs, err.Error())
		}

		if err := plugin.Drain(types.RawMessage(pluginSpec.Properties), inst); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.New(strings.Join(errs, ", "))
}

func cloneSpec(spec instance.Spec) instance.Spec {
	tags := map[string]string{}
	for k, v := range spec.Tags {
		tags[k] = v
	}

	var logicalID *instance.LogicalID
	if spec.LogicalID != nil {
		idCopy := *spec.LogicalID
		logicalID = &idCopy
	}

	attachments := []instance.Attachment{}
	for _, v := range spec.Attachments {
		attachments = append(attachments, v)
	}

	return instance.Spec{
		Properties:  spec.Properties,
		Tags:        tags,
		Init:        spec.Init,
		LogicalID:   logicalID,
		Attachments: attachments,
	}
}

func mergeSpecs(initial instance.Spec, specs []instance.Spec) (instance.Spec, error) {
	result := cloneSpec(initial)

	for _, spec := range specs {
		for k, v := range spec.Tags {
			result.Tags[k] = v
		}

		if spec.Init != "" {
			if result.Init != "" {
				result.Init += "\n"
			}

			result.Init += spec.Init
		}

		for _, v := range spec.Attachments {
			result.Attachments = append(result.Attachments, v)
		}
	}

	return result, nil
}

func (f flavorCombo) Prepare(
	flavor json.RawMessage,
	inst instance.Spec,
	allocation types.AllocationMethod) (instance.Spec, error) {

	combo := Spec{}
	err := json.Unmarshal(flavor, &combo)
	if err != nil {
		return inst, err
	}

	specs := []instance.Spec{}
	for _, pluginSpec := range combo.Flavors {
		// Copy the instance spec to prevent Flavor plugins from interfering with each other.
		clone := cloneSpec(inst)

		plugin, err := f.flavorPlugins(pluginSpec.Plugin)
		if err != nil {
			return inst, err
		}

		flavorOutput, err := plugin.Prepare(types.RawMessage(pluginSpec.Properties), clone, allocation)
		if err != nil {
			return inst, err
		}
		specs = append(specs, flavorOutput)
	}

	return mergeSpecs(inst, specs)
}
