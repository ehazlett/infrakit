package flavor

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/docker/infrakit/pkg/spi"
	"github.com/docker/infrakit/pkg/spi/flavor"
)

// PluginServer returns a Flavor that conforms to the net/rpc rpc call convention.
func PluginServer(p flavor.Plugin) *Flavor {
	return &Flavor{plugin: p}
}

// PluginServerWithTypes which supports multiple types of flavor plugins. The de-multiplexing
// is done by the server's RPC method implementations.
func PluginServerWithTypes(typed map[string]flavor.Plugin) *Flavor {
	return &Flavor{typedPlugins: typed}
}

// Flavor the exported type needed to conform to json-rpc call convention
type Flavor struct {
	plugin       flavor.Plugin
	typedPlugins map[string]flavor.Plugin // by type, as qualified in the name of the plugin
}

// VendorInfo returns a metadata object about the plugin, if the plugin implements it.  See spi.Vendor
func (p *Flavor) VendorInfo() *spi.VendorInfo {
	// TODO(chungers) - support typed plugins
	if p.plugin == nil {
		return nil
	}

	if m, is := p.plugin.(spi.Vendor); is {
		return m.VendorInfo()
	}
	return nil
}

// SetExampleProperties sets the rpc request with any example properties/ custom type
func (p *Flavor) SetExampleProperties(request interface{}) {
	// TODO(chungers) - support typed plugins
	if p.plugin == nil {
		return
	}

	i, is := p.plugin.(spi.InputExample)
	if !is {
		return
	}
	example := i.ExampleProperties()
	if example == nil {
		return
	}

	switch request := request.(type) {
	case *PrepareRequest:
		request.Properties = example
	case *HealthyRequest:
		request.Properties = example
	case *DrainRequest:
		request.Properties = example
	}
}

// exampleProperties returns an example properties used by the plugin
func (p *Flavor) exampleProperties() *json.RawMessage {
	if i, is := p.plugin.(spi.InputExample); is {
		return i.ExampleProperties()
	}
	return nil
}

// ImplementedInterface returns the interface implemented by this RPC service.
func (p *Flavor) ImplementedInterface() spi.InterfaceSpec {
	return flavor.InterfaceSpec
}

func (p *Flavor) getPlugin(flavorType string) flavor.Plugin {
	if flavorType == "" {
		return p.plugin
	}
	if p, has := p.typedPlugins[flavorType]; has {
		return p
	}
	return nil
}

// Validate checks whether the helper can support a configuration.
func (p *Flavor) Validate(_ *http.Request, req *ValidateRequest, resp *ValidateResponse) error {
	var raw json.RawMessage
	if req.Properties != nil {
		raw = *req.Properties
	}

	resp.Type = req.Type
	c := p.getPlugin(req.Type)
	if c == nil {
		return fmt.Errorf("no-plugin:%s", req.Type)
	}
	err := c.Validate(raw, req.Allocation)
	if err != nil {
		return err
	}
	resp.OK = true
	return nil
}

// Prepare allows the Flavor to modify the provisioning instructions for an instance.  For example, a
// helper could be used to place additional tags on the machine, or generate a specialized Init command based on
// the flavor configuration.
func (p *Flavor) Prepare(_ *http.Request, req *PrepareRequest, resp *PrepareResponse) error {
	var raw json.RawMessage
	if req.Properties != nil {
		raw = *req.Properties
	}

	resp.Type = req.Type
	c := p.getPlugin(req.Type)
	if c == nil {
		return fmt.Errorf("no-plugin:%s", req.Type)
	}
	spec, err := c.Prepare(raw, req.Spec, req.Allocation)
	if err != nil {
		return err
	}
	resp.Spec = spec
	return nil
}

// Healthy determines whether an instance is healthy.
func (p *Flavor) Healthy(_ *http.Request, req *HealthyRequest, resp *HealthyResponse) error {
	resp.Type = req.Type
	c := p.getPlugin(req.Type)
	if c == nil {
		return fmt.Errorf("no-plugin:%s", req.Type)
	}
	health, err := c.Healthy(*req.Properties, req.Instance)
	if err != nil {
		return err
	}
	resp.Health = health
	return nil
}

// Drain drains the instance. It's the inverse of prepare before provision and happens before destroy.
func (p *Flavor) Drain(_ *http.Request, req *DrainRequest, resp *DrainResponse) error {
	resp.Type = req.Type
	c := p.getPlugin(req.Type)
	if c == nil {
		return fmt.Errorf("no-plugin:%s", req.Type)
	}
	err := c.Drain(*req.Properties, req.Instance)
	if err != nil {
		return err
	}
	resp.OK = true
	return nil
}
