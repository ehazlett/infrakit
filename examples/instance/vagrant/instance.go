package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/docker/infrakit/pkg/spi/instance"
	"github.com/docker/infrakit/pkg/template"
)

// NewVagrantPlugin creates an instance plugin for vagrant.
func NewVagrantPlugin(dir string, template *template.Template) instance.Plugin {
	return &vagrantPlugin{VagrantfilesDir: dir, VagrantTmpl: template}
}

type vagrantPlugin struct {
	VagrantfilesDir string
	VagrantTmpl     *template.Template
}

// Validate performs local validation on a provision request.
func (v vagrantPlugin) Validate(req json.RawMessage) error {
	return nil
}

func inheritedEnvCommand(cmdAndArgs []string, extraEnv ...string) (string, error) {
	cmd := exec.Command(cmdAndArgs[0], cmdAndArgs[1:]...)
	cmd.Env = append(os.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	fmt.Printf("DEBUGGING cmd output: %s\n", string(output))
	if err != nil {
		fmt.Printf("Err: %s\n", err)
	}
	return string(output), err
}

// Provision creates a new instance.
func (v vagrantPlugin) Provision(spec instance.Spec) (*instance.ID, error) {

	var properties map[string]interface{}

	if spec.Properties != nil {
		if err := json.Unmarshal(*spec.Properties, &properties); err != nil {
			return nil, fmt.Errorf("Invalid instance properties: %s", err)
		}
	}

	if properties["Box"] == nil {
		return nil, errors.New("Property 'Box' must be set")
	}
	if properties["CPUs"] == nil {
		properties["CPUs"] = 2
	}
	if properties["Memory"] == nil {
		properties["Memory"] = 512
	}

	networkOptions := `, type: "dhcp"`
	if spec.LogicalID != nil {
		networkOptions = fmt.Sprintf(`, ip: "%s"`, *spec.LogicalID)
	}

	config := bytes.Buffer{}

	params := map[string]interface{}{
		"NetworkOptions": networkOptions,
		"Properties":     properties,
	}
	if err := v.VagrantTmpl.Execute(&config, params); err != nil {
		return nil, err
	}

	machineDir, err := ioutil.TempDir(v.VagrantfilesDir, "infrakit-")
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(path.Join(machineDir, "boot.sh"), []byte(spec.Init), 0755); err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(path.Join(machineDir, "Vagrantfile"), config.Bytes(), 0666); err != nil {
		return nil, err
	}

	id := instance.ID(path.Base(machineDir))

	_, err = inheritedEnvCommand([]string{"vagrant", "up"}, fmt.Sprintf("VAGRANT_CWD=%s", machineDir))
	if err != nil {
		v.Destroy(id)
		return nil, err
	}

	tagData, err := json.Marshal(spec.Tags)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(path.Join(machineDir, "tags"), tagData, 0666); err != nil {
		return nil, err
	}

	if spec.LogicalID != nil {
		if err := ioutil.WriteFile(path.Join(machineDir, "ip"), []byte(*spec.LogicalID), 0666); err != nil {
			return nil, err
		}
	}

	return &id, nil
}

// Destroy terminates an existing instance.
func (v vagrantPlugin) Destroy(id instance.ID) error {
	fmt.Println("Destroying ", id)

	machineDir := path.Join(v.VagrantfilesDir, string(id))
	_, err := os.Stat(machineDir)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("Instance does not exist")
		}
	}

	_, err = inheritedEnvCommand([]string{"vagrant", "destroy", "-f"}, fmt.Sprintf("VAGRANT_CWD=%s", machineDir))
	if err != nil {
		fmt.Println("Vagrant destroy failed: ", err)
	}

	if err := os.RemoveAll(machineDir); err != nil {
		return err
	}

	return nil
}

// DescribeInstances returns descriptions of all instances matching all of the provided tags.
func (v vagrantPlugin) DescribeInstances(tags map[string]string) ([]instance.Description, error) {
	files, err := ioutil.ReadDir(v.VagrantfilesDir)
	if err != nil {
		return nil, err
	}

	descriptions := []instance.Description{}

	for _, file := range files {
		if !file.IsDir() {
			continue
		}

		machineDir := path.Join(v.VagrantfilesDir, file.Name())

		tagData, err := ioutil.ReadFile(path.Join(machineDir, "tags"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return nil, err
		}

		machineTags := map[string]string{}
		if err := json.Unmarshal(tagData, &machineTags); err != nil {
			return nil, err
		}

		allMatched := true
		for k, v := range tags {
			value, exists := machineTags[k]
			if !exists || v != value {
				allMatched = false
				break
			}
		}

		if allMatched {
			var logicalID *instance.LogicalID
			ipData, err := ioutil.ReadFile(path.Join(machineDir, "ip"))
			if err == nil {
				id := instance.LogicalID(ipData)
				logicalID = &id
			} else {
				if !os.IsNotExist(err) {
					return nil, err
				}
			}

			descriptions = append(descriptions, instance.Description{
				ID:        instance.ID(file.Name()),
				LogicalID: logicalID,
				Tags:      machineTags,
			})
		}
	}

	return descriptions, nil
}
