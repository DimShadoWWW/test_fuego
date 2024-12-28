package oapi

import (
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
)

type customOpenAPI struct {
	*fuego.OpenAPI
}

func (o *customOpenAPI) buildOpenapi3Response(description string, response fuego.Response) *openapi3.Response {
	if response.Type == nil {
		panic("Type in Response cannot be nil")
	}

	responseSchema := SchemaTagFromType(o, response.Type)
	if len(response.ContentTypes) == 0 {
		response.ContentTypes = []string{"application/xml"}
	}

	content := openapi3.NewContentWithSchemaRef(&responseSchema.SchemaRef, response.ContentTypes)
	return openapi3.NewResponse().
		WithDescription(description).
		WithContent(content)
}

type customBaseRoute struct {
	fuego.BaseRoute

	OpenAPI *customOpenAPI
}


func CustomOptionAddResponse(code int, description string, response fuego.Response) func(*fuego.BaseRoute) {
	return func(r *fuego.BaseRoute) {
		if r.Operation.Responses == nil {
			r.Operation.Responses = openapi3.NewResponses()
		}

		customRoute := &customBaseRoute{
			BaseRoute: *r,
			OpenAPI:   &customOpenAPI{OpenAPI: r.OpenAPI},
		}
		customRoute.Operation.Responses.Set(
			strconv.Itoa(code), &openapi3.ResponseRef{
				Value: customRoute.OpenAPI.buildOpenapi3Response(description, response),
			},
		)
	}
}

// AddError adds an error to the route.
// It replaces any existing error previously set with the same code.
// Required: should only supply one type to `errorType`
// Deprecated: Use [OptionAddResponse] instead
func OptionAddError(code int, description string, errorType ...any) func(*customBaseRoute) {
	var responseSchema fuego.SchemaTag
	return func(r *customBaseRoute) {
		if len(errorType) > 1 {
			panic("errorType should not be more than one")
		}

		if len(errorType) > 0 {
			responseSchema = SchemaTagFromType(r.OpenAPI, errorType[0])
		} else {
			responseSchema = SchemaTagFromType(r.OpenAPI, fuego.HTTPError{})
		}
		content := openapi3.NewContentWithSchemaRef(&responseSchema.SchemaRef, []string{"application/xml"})

		response := openapi3.NewResponse().
			WithDescription(description).
			WithContent(content)

		if r.Operation.Responses == nil {
			r.Operation.Responses = openapi3.NewResponses()
		}
		r.Operation.Responses.Set(strconv.Itoa(code), &openapi3.ResponseRef{Value: response})
	}
}

func SchemaTagFromType(openapi *customOpenAPI, v any) fuego.SchemaTag {
	if v == nil {
		// ensure we add unknown-interface to our schemas
		schema := openapi.getOrCreateSchema("unknown-interface", struct{}{})
		return fuego.SchemaTag{
			Name: "unknown-interface",
			SchemaRef: openapi3.SchemaRef{
				Ref:   "#/components/schemas/unknown-interface",
				Value: schema,
			},
		}
	}

	return dive(openapi, reflect.TypeOf(v), fuego.SchemaTag{}, 5)
}

// dive returns a schemaTag which includes the generated openapi3.SchemaRef and
// the name of the struct being passed in.
// If the type is a pointer, map, channel, function, or unsafe pointer,
// it will dive into the type and return the name of the type it points to.
// If the type is a slice or array type it will dive into the type as well as
// build and openapi3.Schema where Type is array and Ref is set to the proper
// components Schema
func dive(openapi *customOpenAPI, t reflect.Type, tag fuego.SchemaTag, maxDepth int) fuego.SchemaTag {
	if maxDepth == 0 {
		return fuego.SchemaTag{
			Name: "default",
			SchemaRef: openapi3.SchemaRef{
				Ref: "#/components/schemas/default",
			},
		}
	}

	switch t.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return dive(openapi, t.Elem(), tag, maxDepth-1)

	case reflect.Slice, reflect.Array:
		item := dive(openapi, t.Elem(), tag, maxDepth-1)
		tag.Name = item.Name
		tag.Value = openapi3.NewArraySchema()
		tag.Value.Items = &item.SchemaRef
		return tag

	default:
		tag.Name = t.Name()
		if t.Kind() == reflect.Struct && strings.HasPrefix(tag.Name, "DataOrTemplate") {
			return dive(openapi, t.Field(0).Type, tag, maxDepth-1)
		}
		tag.Ref = "#/components/schemas/" + tag.Name
		tag.Value = openapi.getOrCreateSchema(tag.Name, reflect.New(t).Interface())

		return tag
	}
}

// getOrCreateSchema is used to get a schema from the OpenAPI spec.
// If the schema does not exist, it will create a new schema and add it to the OpenAPI spec.
func (openapi *customOpenAPI) getOrCreateSchema(key string, v any) *openapi3.Schema {
	schemaRef, ok := openapi.Description().Components.Schemas[key]
	if !ok {
		schemaRef = openapi.createSchema(key, v)
	}
	return schemaRef.Value
}

// createSchema is used to create a new schema and add it to the OpenAPI spec.
// Relies on the openapi3gen package to generate the schema, and adds custom struct tags.
func (openapi *customOpenAPI) createSchema(key string, v any) *openapi3.SchemaRef {
	schemaRef, err := openapi.Generator().NewSchemaRefForValue(v, openapi.Description().Components.Schemas)
	if err != nil {
		slog.Error("Error generating schema", "key", key, "error", err)
	}
	schemaRef.Value.Description = key + " schema"

	descriptionable, ok := v.(fuego.OpenAPIDescriptioner)
	if ok {
		schemaRef.Value.Description = descriptionable.Description()
	}

	parseStructTags(reflect.TypeOf(v), schemaRef)

	openapi.Description().Components.Schemas[key] = schemaRef

	return schemaRef
}

// parseStructTags parses struct tags and modifies the schema accordingly.
// t must be a struct type.
// It adds the following struct tags (tag => OpenAPI schema field):
// - description => description
// - example => example
// - json => nullable (if contains omitempty)
// - validate:
//   - required => required
//   - min=1 => min=1 (for integers)
//   - min=1 => minLength=1 (for strings)
//   - max=100 => max=100 (for integers)
//   - max=100 => maxLength=100 (for strings)
func parseStructTags(t reflect.Type, schemaRef *openapi3.SchemaRef) {
	fmt.Println("parseStructTags")

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return
	}

	for i := range t.NumField() {
		field := t.Field(i)

		if field.Anonymous {
			fieldType := field.Type
			parseStructTags(fieldType, schemaRef)
			continue
		}

		jsonFieldName := field.Tag.Get("json")
		jsonFieldName = strings.Split(jsonFieldName, ",")[0] // remove omitempty, etc
		if jsonFieldName == "-" {
			continue
		}
		if jsonFieldName == "" {
			jsonFieldName = field.Name
		}

		property := schemaRef.Value.Properties[jsonFieldName]
		if property == nil {
			slog.Warn("Property not found in schema", "property", jsonFieldName)
			continue
		}
		propertyCopy := *property
		propertyValue := *propertyCopy.Value

		// Xml attributes
		xmlTag, ok := field.Tag.Lookup("xml")
		if ok {
			fmt.Println("xmlTag", xmlTag)
			xmlTagName := strings.Split(xmlTag, ",")[0] // remove omitempty, etc
			if xmlTagName == "-" {
				continue
			}
			if xmlTagName == "" {
				xmlTagName = field.Name
			}
			fmt.Println("xmlTagName", xmlTagName)

			propertyValue.XML = &openapi3.XML{
				Name: xmlTagName,
			}

			xmlTags := strings.Split(xmlTag, ",")
			if ok && slices.Contains(xmlTags, "attr") {
				propertyValue.XML.Attribute = true
			}
		}

		// Example
		example, ok := field.Tag.Lookup("example")
		if ok {
			propertyValue.Example = example
			if propertyValue.Type.Is(openapi3.TypeInteger) {
				exNum, err := strconv.Atoi(example)
				if err != nil {
					slog.Warn("Example might be incorrect (should be integer)", "error", err)
				}
				propertyValue.Example = exNum
			}
		}

		// Validation
		validateTag, ok := field.Tag.Lookup("validate")
		validateTags := strings.Split(validateTag, ",")
		if ok && slices.Contains(validateTags, "required") {
			schemaRef.Value.Required = append(schemaRef.Value.Required, jsonFieldName)
		}
		for _, validateTag := range validateTags {
			if strings.HasPrefix(validateTag, "min=") {
				min, err := strconv.Atoi(strings.Split(validateTag, "=")[1])
				if err != nil {
					slog.Warn("Min might be incorrect (should be integer)", "error", err)
				}

				if propertyValue.Type.Is(openapi3.TypeInteger) {
					minPtr := float64(min)
					propertyValue.Min = &minPtr
				} else if propertyValue.Type.Is(openapi3.TypeString) {
					//nolint:gosec // disable G115
					propertyValue.MinLength = uint64(min)
				}
			}
			if strings.HasPrefix(validateTag, "max=") {
				max, err := strconv.Atoi(strings.Split(validateTag, "=")[1])
				if err != nil {
					slog.Warn("Max might be incorrect (should be integer)", "error", err)
				}
				if propertyValue.Type.Is(openapi3.TypeInteger) {
					maxPtr := float64(max)
					propertyValue.Max = &maxPtr
				} else if propertyValue.Type.Is(openapi3.TypeString) {
					//nolint:gosec // disable G115
					maxPtr := uint64(max)
					propertyValue.MaxLength = &maxPtr
				}
			}
		}

		// Description
		description, ok := field.Tag.Lookup("description")
		if ok {
			propertyValue.Description = description
		}
		jsonTag, ok := field.Tag.Lookup("json")
		if ok {
			if strings.Contains(jsonTag, ",omitempty") {
				propertyValue.Nullable = true
			}
		}
		propertyCopy.Value = &propertyValue

		schemaRef.Value.Properties[jsonFieldName] = &propertyCopy
	}
}
