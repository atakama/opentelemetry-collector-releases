package pdata

// Ajout des sementiques non définies
var (
	SemanticAttributesX = map[string]string{
		"HTTP_HOST_IP":                 "http.host.ip",
		"NET_HOST":                     "net.host",
		"NET_PEER":                     "net.peer",
		"HTTP_ERROR_NAME":              "http.error_name",
		"HTTP_ERROR_MESSAGE":           "http.error_message",
		"HTTP_STATUS_TEXT":             "http.status_text",
		"HTTP_STACKTRACE":              "http.stacktrace",
		"LAYER_TYPE_SQL":               "SQL",
		"LAYER_TYPE_HTTP":              "HTTP",
		"HTTP_RESPONSE_CONTENT_TYPE":   "http.response.header.content_type",
		"HTTP_REQUEST_USER_AGENT":      "http.request.header.user_agent",
		"HTTP_RESPONSE_USER_AGENT":     "http.user_agent",
		"HTTP_EXECUTION_CLIENT":        "http.execution_client",
		"HTTP_REQUEST_X_FORWARDED_FOR": "http.request.header.x_forwarded_for",
		"HTTP_RESPONSE_X_POWERED_BY":   "http.response.header.x_powered_by",
		"UUID":                         "UUID",

		"EXPRESS_TYPE": "express.type",
		"EXPRESS_NAME": "express.name",
	}
)
