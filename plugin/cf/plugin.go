// The `cf` plugin for SHIELD implements generic backup + restore
// functionality for running Cloud Foundry applications.
//
// The backup function will extract a CF app's droplet + metadata necessary
// to allow the application to be recreated in future.
// The restore function allows the application to be recreated in the future.
// This would probably coincide the restoration of the application's data services.
//
// PLUGIN FEATURES
//
// This plugin implements functionality suitable for use with the following
// SHIELD Job components:
//
//    Target: yes
//    Store:  no
//
// PLUGIN CONFIGURATION
//
// The endpoint configuration passed to this plugin is used to identify what
// Cloud Foundry API to target, and which org/space/app to backup/restore.
// Your endpoint JSON should look something like this:
//
//    {
//        "api_url":  "https://api.mycf.com",
//        "username": "myname@email.com",
//        "password": "password",
//        "skip_ssl_validation": false,
//        "organization": "my-org",
//        "space": "production",
//        "appname": "myapp",
//        "cf_bin": "/path/to/bin/cf"
//    }
//
// Default Configuration
//
//    {
//        "skip_ssl_verification": false,
//        "cf_bin": "/usr/local/bin/cf"
//    }
//
// BACKUP DETAILS
//
// The `cf` plugin uses the `cf` CLI to download an application's droplet
// and learn its runtime environment.
//
// RESTORE DETAILS
//
// The `cf` plugin uses the `cf` CLI to upload an application's droplet
// and recreate its runtime environment.
//
// DEPENDENCIES
//
// This plugin relies on the `cf` CLI from https://github.com/cloudfoundry/cli.
// Please ensure that it is present on the system that will be running the
// backups + restores. If you are using shield-boshrelease,
// this is provided automatically for you as part of the `shield-agent` job template.
//
package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/starkandwayne/goutils/ansi"

	"github.com/starkandwayne/shield/plugin"
)

var (
	DefaultCfBin             = "/usr/local/bin/cf"
	DefaultSkipSSLValidation = false
)

func main() {
	p := CFPlugin{
		Name:    "Cloud Foundry Plugin",
		Author:  "Stark & Wayne",
		Version: "1.0.0",
		Features: plugin.PluginFeatures{
			Target: "yes",
			Store:  "no",
		},
		Example: `
{
    "api_url":  "https://api.mycf.com",
    "username": "myname@email.com",
    "password": "password",
    "skip_ssl_validation": false,

    "organization": "my-org",
    "space":        "production",
    "appname":      "myapp",

    "cf_bin": "/path/to/bin/cf"
}
`,
		Defaults: `
{
    "skip_ssl_verification": false,
    "cf_bin": "/var/vcap/packages/cf_cli/bin/cf"
}
`,
	}

	plugin.DEBUG("cf plugin starting up...")
	plugin.Run(p)
}

type CFPlugin plugin.PluginInfo

type CFConfig struct {
	APIURL            string `json:"api_url"`
	SkipSSLValidation bool   `json:"skip_ssl_validation"`
	Username          string `json:"username"`
	Password          string `json:"password"`

	Organization string `json:"organization"`
	Space        string `json:"space"`
	AppName      string `json:"appname"`

	CfBin string `json:"cf_bin"`
}

func (p CFPlugin) Meta() plugin.PluginInfo {
	return plugin.PluginInfo(p)
}

func getCFConfig(endpoint plugin.ShieldEndpoint) (config *CFConfig, err error) {
	config = &CFConfig{}

	config.APIURL, err = endpoint.StringValue("api_url")
	if err != nil {
		return
	}

	config.Username, err = endpoint.StringValue("username")
	if err != nil {
		return nil, err
	}

	config.Password, err = endpoint.StringValue("password")
	if err != nil {
		return nil, err
	}

	config.Organization, err = endpoint.StringValue("organization")
	if err != nil {
		return nil, err
	}

	config.Space, err = endpoint.StringValue("space")
	if err != nil {
		return nil, err
	}

	config.AppName, err = endpoint.StringValue("app_name")
	if err != nil {
		return nil, err
	}

	config.CfBin, err = endpoint.StringValueDefault("cf_bin", DefaultCfBin)
	if err != nil {
		return nil, err
	}

	config.SkipSSLValidation, err = endpoint.BooleanValueDefault("skip_ssl_validation", DefaultSkipSSLValidation)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (p CFPlugin) Validate(endpoint plugin.ShieldEndpoint) error {
	var (
		s    string
		err  error
		fail bool
	)

	requiredConfig := []string{"api_url", "username", "password", "organization", "space", "appname"}
	for _, reqConfig := range requiredConfig {
		s, err = endpoint.StringValue(reqConfig)
		if err != nil {
			ansi.Printf("@R{\u2717 %s   %s}\n", reqConfig, err)
			fail = true
		} else {
			ansi.Printf("@G{\u2713 %s}   @C{%s}\n", reqConfig, s)
		}
	}

	if fail {
		return fmt.Errorf("cf: invalid configuration")
	}
	return nil
}

func (p CFPlugin) login(cfg *CFConfig) error {
	script := `#!/bin/bash

echo "Running $@"
api_url=$1; shift
skip_ssl_validation=$1; shift
username=$1; shift
password=$1; shift
organization=$1; shift
space=$1; shift
appname=$1; shift
cf_bin=$1; shift
if [[ "${cf_bin:-X}" == "X" ]]; then
  echo "Missing arguments"
  exit 1
fi

skip_flag=
if [[ "${skip_ssl_validation}" == "true" ]]; then
  skip_flag=" --skip-ssl-validation"
fi
$cf_bin login -a $api_url -u $username -p $password -o $organization -s $space $skip_flag

app_guid=$($cf_bin app --guid $appname)
echo "App GUID: $app_guid"

$cf_bin curl /v2/apps/${app_guid}
`
	// file, err := ioutil.TempFile("tmp", "cf-actions")
	file, err := ioutil.TempFile(os.TempDir(), "cf-actions")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write([]byte(script)); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), 0755); err != nil {
		return err
	}

	cmd := fmt.Sprintf("%s", file.Name())
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.APIURL)
	cmd = fmt.Sprintf("%s '%v'", cmd, cfg.SkipSSLValidation)
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.Username)
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.Password)
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.Organization)
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.Space)
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.AppName)
	cmd = fmt.Sprintf("%s '%s'", cmd, cfg.CfBin)
	plugin.DEBUG("Login: executing `%s`", file.Name())
	return plugin.Exec(cmd, plugin.STDOUT)
}

func (p CFPlugin) Backup(endpoint plugin.ShieldEndpoint) error {
	cfg, err := getCFConfig(endpoint)
	if err != nil {
		return err
	}

	p.login(cfg)

	return nil
}

func (p CFPlugin) Restore(endpoint plugin.ShieldEndpoint) error {
	cfg, err := getCFConfig(endpoint)
	if err != nil {
		return err
	}

	p.login(cfg)

	return nil
}

func (p CFPlugin) Store(endpoint plugin.ShieldEndpoint) (string, error) {
	return "", plugin.UNIMPLEMENTED
}

func (p CFPlugin) Retrieve(endpoint plugin.ShieldEndpoint, file string) error {
	return plugin.UNIMPLEMENTED
}

func (p CFPlugin) Purge(endpoint plugin.ShieldEndpoint, file string) error {
	return plugin.UNIMPLEMENTED
}
