package cmd

func prefixAIActionNames(actions []string, includeBang bool) []string {
	prefixed := make([]string, 0, len(actions))
	for _, action := range actions {
		if includeBang {
			prefixed = append(prefixed, "!"+action)
			continue
		}
		prefixed = append(prefixed, action)
	}

	return prefixed
}
