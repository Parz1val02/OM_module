package collector

// ActiveGeneration inspects the current snapshot and returns the single
// generation ("4g" or "5g") that has running core-domain containers.
//
// Rules:
//   - Only containers with om.domain == "core" and state == "running" are considered.
//   - If exactly one distinct generation is found, it is returned.
//   - If zero or multiple distinct generations are found, "" is returned.
//     The capture manager interprets "" as "not ready yet" and retries.
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
