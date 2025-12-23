package utils

import "strings"

// ContainsNF checks if component name contains network function name
func ContainsNF(componentName, nfName string) bool {
	return strings.Contains(strings.ToLower(componentName), nfName)
}

// GetCollectorPort returns the port for a given NF type
func GetCollectorPort(nfType string) string {
	ports := map[string]string{
		"amf": "9091", "smf": "9092", "pcf": "9093",
		"upf": "9094", "mme": "9095", "pcrf": "9096",
	}
	if port, exists := ports[nfType]; exists {
		return port
	}
	return "9091" // default
}
