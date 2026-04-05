package collector

// ActiveGeneration inspects the current snapshot and returns the single
// generation ("4g" or "5g") that has running core-domain containers.
func (s *Snapshot) ActiveGeneration() string {
	all := s.All()

	generations := make(map[string]bool)
	for _, cd := range all {
		if cd.Domain == DomainCore && cd.State == "running" && cd.Generation != "" {
			generations[cd.Generation] = true
		}
	}

	if len(generations) == 1 {
		for gen := range generations {
			return gen
		}
	}

	return ""
}

// NFByName returns the om.nf label value for a container with the given name.
// Returns the container name itself if no om.nf label is found, and "" if
// no container with that name exists in the snapshot.
func (s *Snapshot) NFByName(containerName string) string {
	for _, cd := range s.All() {
		if cd.Name == containerName {
			if cd.NF != "" {
				return cd.NF
			}
			return containerName
		}
	}
	return ""
}

// NameToNFMap returns a map of container name → om.nf label for all containers
// currently in the snapshot. Used by the correlator to resolve IP → NF name
// by joining with the Docker network IP map.
func (s *Snapshot) NameToNFMap() map[string]string {
	all := s.All()
	result := make(map[string]string, len(all))
	for _, cd := range all {
		nf := cd.NF
		if nf == "" {
			nf = cd.Name
		}
		result[cd.Name] = nf
	}
	return result
}
