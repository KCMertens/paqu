package main

func init() {

	optionsHandler := func(q *Context) {
		q.w.Header().Set("Access-Control-Allow-Origin", "*")
		q.w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		q.w.Header().Set("Access-Control-Allow-Headers", "Accept, Accept-Language, Content-Language, Content-Type")
	}

	// dynamic handlers krijgen een complete context
	localDynamicHandlers = append(localDynamicHandlers,
		LocalHandlerType{"api/gretel/results", api_gretel_results, &HandlerOptions{false, optionsHandler}},
		LocalHandlerType{"api/gretel/configured_treebanks", api_gretel_configured_treebanks, &HandlerOptions{false, optionsHandler}},
		LocalHandlerType{"api/gretel/metadata_counts", api_gretel_metadata_counts, &HandlerOptions{false, optionsHandler}},
		LocalHandlerType{"api/gretel/treebank_counts", api_gretel_treebank_counts, &HandlerOptions{false, optionsHandler}},
		LocalHandlerType{"api/gretel/tree/", api_gretel_highlight_tree, &HandlerOptions{false, optionsHandler}})
}
