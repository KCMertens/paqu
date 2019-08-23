package main

func init() {

	optionsHandler := func(q *Context) {
		q.w.Header().Set("Access-Control-Allow-Origin", "*")
		q.w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		q.w.Header().Set("Access-Control-Allow-Headers", "Accept, Accept-Language, Content-Language, Content-Type")
	}

	handlerSettings := HandlerOptions{
		NeedForm:             false,
		OptionsMethodHandler: optionsHandler,
	}

	// dynamic handlers krijgen een complete context
	localDynamicHandlers = append(localDynamicHandlers,
		LocalHandlerType{"api/gretel/results", api_gretel_results, &handlerSettings},
		LocalHandlerType{"api/gretel/configured_treebanks", api_gretel_configured_treebanks, &handlerSettings},
		LocalHandlerType{"api/gretel/metadata_counts", api_gretel_metadata_counts, &handlerSettings},
		LocalHandlerType{"api/gretel/treebank_counts", api_gretel_treebank_counts, &handlerSettings},
		LocalHandlerType{"api/gretel/tree/", api_gretel_show_tree, &handlerSettings})
}
