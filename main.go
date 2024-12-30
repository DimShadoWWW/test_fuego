package main

import (
	"encoding/xml"
	"fmt"
	"net/http"

	// "testapi/api/oapi"

	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"
	"github.com/go-fuego/fuego/param"
)

type Health struct {
	Status string `json:"status" xml:"Status,attr" example:"ok"`

	Input MyInput `json:"input" xml:"input"`
}

type MyInput struct {
	Name   string    `json:"name" xml:"name,attr" validate:"required"  example:"Hello, Carmack"`
	Values []MyValue `json:"values" xml:"values"`
}

type MyValue struct {
	Value string `json:"value" xml:"value,attr" example:"example value"`
}

type MyOutput struct {
	XMLName xml.Name `xml:"Output"`

	Data    string  `json:"data" xml:"data,attr" example:"example data"`
	Message MyInput `json:"message" xml:"message"`
}

func openAPIHandler(specURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// publicURL.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Swagger UI</title>
        <link rel="stylesheet" type="text/css" href="https://petstore.swagger.io/swagger-ui.css" />
    <link rel="stylesheet" type="text/css" href="https://petstore.swagger.io/index.css" />
    <link rel="icon" type="image/png" href="https://petstore.swagger.io/favicon-32x32.png" sizes="32x32" />
    <link rel="icon" type="image/png" href="https://petstore.swagger.io/favicon-16x16.png" sizes="16x16" />
</head>
<body style="height: 100vh;">
        <div id="swagger-ui"></div>
    <script src="https://petstore.swagger.io/swagger-ui-bundle.js" charset="UTF-8"> </script>
    <script src="https://petstore.swagger.io/swagger-ui-standalone-preset.js" charset="UTF-8"> </script>
    <script charset="UTF-8">
        window.onload = function() {
        // the following lines will be replaced by docker/configurator, when it runs in a docker-container
        window.ui = SwaggerUIBundle({
                url: "/swagger/openapi.json",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
                ],
                plugins: [
                SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
        });
        };
        </script>
</body></html>`))
	})
}

func main() {
	s := fuego.NewServer(
		fuego.WithAddr("0.0.0.0:8080"),
		fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
			UIHandler: openAPIHandler,
		}),
		// fuego.WithRouteOptions(
		// 	oapi.CustomOptionAddResponse(http.StatusOK, "Health check", fuego.Response{
		// 		ContentTypes: []string{"application/json"},
		// 		Type:         Health{},
		// 	}),
		// ),
	)

	s.Engine.OpenAPI.Description().Info.Title = "My API"
	s.Engine.OpenAPI.Description().Info.Version = "1.0.0"
	s.Engine.OpenAPI.Description().Info.Description = "This is a sample server for Fuego."
	// s.Engine.OpenAPI.Generator().GenerateSchemaRef()

	fuego.Get(s, "/health", func(c fuego.ContextNoBody) (Health, error) {
		return Health{Status: "ok", Input: MyInput{Name: "Carmack"}}, nil
	}, option.Description("Health check"),
		option.Summary("Returns a status message"),
		option.OperationID("health"),
		option.Tags("health"),
		// oapi.CustomOptionAddResponse(http.StatusOK, "Health check", fuego.Response{
		// 	ContentTypes: []string{"application/xml", "application/json"},
		// 	Type:         Health{},
		// }),
	)

	// Automatically generates OpenAPI documentation for this route
	fuego.Post(s, "/user/{user}", myController,
		option.Description("This route does something..."),
		option.Summary("This is my summary"),
		option.Tags("MyTag"), // A tag is set by default according to the return type (can be deactivated)
		// option.Deprecated(),  // Marks the route as deprecated in the OpenAPI spec
		option.Query("name", "Declares a query parameter with default value", param.Default("Carmack")),
		option.Header("Authorization", "Bearer token", param.Required()),
		optionPagination,
		// oapi.CustomOptionAddResponse(http.StatusOK, "Health check", fuego.Response{
		// 	ContentTypes: []string{"application/xml", "application/json"},
		// 	Type:         Health{},
		// }),
	)

	s.Run()
}

func myController(c fuego.ContextWithBody[MyInput]) (MyOutput, error) {
	body, err := c.Body()
	if err != nil {
		return MyOutput{}, err
	}

	return MyOutput{
		Data: "data1",
		Message: MyInput{
			Name: fmt.Sprintf("Hello, %s", body.Name),
			Values: []MyValue{
				{
					Value: "value1",
				},
				{
					Value: "value2",
				},
			},
		},
	}, nil
}

var optionPagination = option.Group(
	option.QueryInt("page", "Page number", param.Default(1), param.Example("1st page", 1), param.Example("42nd page", 42)),
	option.QueryInt("perPage", "Number of items per page"),
)
