// Canonical source: github.com/kubevela/workflow charts/vela-workflow/config-templates/workflow-http-deny.cue
// vela-core uses its own ConfigTemplate name/label (vela-core-http-deny) so dual
// install with vela-workflow does not share Helm ownership of the template.
import (
	"net"
	"strings"
)

metadata: {
	name:        "vela-core-http-deny"
	alias:       "Workflow HTTP Denylist"
	description: "Additional host, IP, and CIDR destinations denied for workflow HTTP requests."
	scope:       "system"
	sensitive:   false
}

#IPAddress: string & net.IP
#IPCIDR:    string & net.IPCIDR

#Hostname:         string & =~"^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)(\\.([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?))*\\.?$"
#WildcardHostname: string & =~"^\\*\\.([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)(\\.([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?))*\\.?$"
#DenyHost:         #IPAddress | #Hostname | #WildcardHostname
#DenyCIDR:         #IPAddress | #IPCIDR

template: {
	outputs: configMap: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      context.name
			namespace: context.namespace
			labels: {
				"config.oam.dev/type":    "vela-core-http-deny"
				"config.oam.dev/scope":   "system"
			}
		}
		data: {
			denyHosts: strings.Join(parameter.denyHosts, "\n")
			denyCIDRs: strings.Join(parameter.denyCIDRs, "\n")
		}
	}
	parameter: {
		// +usage=Exact hostnames, IP addresses, or leading wildcard hostnames to deny.
		// Defaults ship via the Helm default ConfigMap and the controller builtin floor;
		// non-empty CUE list defaults break ConfigTemplate OpenAPI schema generation.
		denyHosts: *[] | [...#DenyHost]
		// +usage=IP addresses or CIDR ranges to deny.
		denyCIDRs: *[] | [...#DenyCIDR]
	}
}
