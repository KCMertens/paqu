package main

func init() {
	// dynamic handlers krijgen een complete context
	localDynamicHandlers = append(localDynamicHandlers,
		LocalHandlerType{"api/gretel/results", api_gretel_results},
		LocalHandlerType{"api/gretel/configured_treebanks", api_gretel_configured_treebanks},
		LocalHandlerType{"api/gretel/metadata_counts", api_gretel_metadata_counts},
		LocalHandlerType{"api/gretel/treebank_counts", api_gretel_treebank_counts},
		LocalHandlerType{"api/gretel/tree/", api_gretel_highlight_tree})
}
