package redirect

import "github.com/coredns/coredns/plugin"

const pluginName = "redirect"

func PluginError(err error) error {
	return plugin.Error(pluginName, err)
}

