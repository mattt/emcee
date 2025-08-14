package internal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"gopkg.in/yaml.v3"
)

// RegisterTools parses the given OpenAPI specification and registers tools on the provided MCP server.
// All HTTP calls are executed using the provided http.Client. If the client is nil, http.DefaultClient is used.
func RegisterTools(server *mcp.Server, specData []byte, client *http.Client) error {
	if len(specData) == 0 {
		return fmt.Errorf("no OpenAPI spec data provided")
	}
	if server == nil {
		return fmt.Errorf("server is nil")
	}
	if client == nil {
		client = http.DefaultClient
	}

	doc, err := libopenapi.NewDocument(specData)
	if err != nil {
		return fmt.Errorf("error parsing OpenAPI spec: %w", err)
	}
	model, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		return fmt.Errorf("error building OpenAPI model: %v", errs[0])
	}

	if len(model.Model.Servers) == 0 || model.Model.Servers[0].URL == "" {
		return fmt.Errorf("OpenAPI spec must include at least one server URL")
	}
	baseURL := strings.TrimSuffix(model.Model.Servers[0].URL, "/")

	// Iterate operations and register tools.
	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil {
		return nil
	}

	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		p := pair.Key()
		item := pair.Value()
		ops := []struct {
			method string
			op     *v3.Operation
		}{
			{"GET", item.Get},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"DELETE", item.Delete},
			{"PATCH", item.Patch},
		}
		for _, op := range ops {
			if op.op == nil || op.op.OperationId == "" {
				continue
			}
			toolName := getToolName(op.op.OperationId)
			desc := op.op.Description
			if desc == "" {
				desc = op.op.Summary
			}

			// Build input schema
			schema := &jsonschema.Schema{Type: "object"}
			schema.Properties = make(map[string]*jsonschema.Schema)

			// Path item parameters
			if item.Parameters != nil {
				for _, param := range item.Parameters {
					addParamToSchema(schema, param)
				}
			}
			// Operation parameters
			if op.op.Parameters != nil {
				for _, param := range op.op.Parameters {
					addParamToSchema(schema, param)
				}
			}
			// Request body (application/json)
			if op.op.RequestBody != nil && op.op.RequestBody.Content != nil {
				if mediaType, ok := op.op.RequestBody.Content.Get("application/json"); ok && mediaType != nil {
					if mediaType.Schema != nil && mediaType.Schema.Schema() != nil {
						if s := mediaType.Schema.Schema(); s.Properties != nil {
							for prop := s.Properties.First(); prop != nil; prop = prop.Next() {
								propName := prop.Key()
								propSchema := prop.Value().Schema()
								if propSchema == nil {
									continue
								}
								sch := &jsonschema.Schema{Type: typeOfSchema(propSchema)}
								sch.Description = buildSchemaDescription("", propSchema)
								schema.Properties[propName] = sch
							}
							if s.Required != nil {
								schema.Required = append(schema.Required, s.Required...)
							}
						}
					}
				}
			}

			tool := &mcp.Tool{
				Name:        toolName,
				Description: desc,
				InputSchema: schema,
			}

			// Capture for handler
			method := op.method
			operation := op.op
			pathItem := item
			pathTemplate := p

			mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[map[string]any]]) (*mcp.CallToolResultFor[any], error) {
				// Build URL
				base, err := url.Parse(baseURL)
				if err != nil {
					return nil, fmt.Errorf("invalid base URL: %w", err)
				}
				p := pathTemplate
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				p = path.Clean(p)
				u := &url.URL{Scheme: base.Scheme, Host: base.Host}
				if base.Path != "" {
					basePath := path.Clean(base.Path)
					u.Path = "/" + strings.TrimPrefix(path.Join(basePath, p), "/")
				} else {
					u.Path = p
				}
				if u.Scheme == "" {
					u.Scheme = "http"
				}

				q := url.Values{}
				headers := make(http.Header)
				var bodyParams map[string]any

				// Path item parameters
				if pathItem.Parameters != nil {
					for _, param := range pathItem.Parameters {
						applyParam(param, req.Params.Arguments, u, q, headers)
					}
				}
				// Operation parameters
				if operation.Parameters != nil {
					for _, param := range operation.Parameters {
						applyParam(param, req.Params.Arguments, u, q, headers)
					}
				}

				// Request body
				if operation.RequestBody != nil && operation.RequestBody.Content != nil {
					if mediaType, ok := operation.RequestBody.Content.Get("application/json"); ok && mediaType != nil {
						if mediaType.Schema != nil && mediaType.Schema.Schema() != nil {
							if s := mediaType.Schema.Schema(); s.Properties != nil {
								bodyParams = make(map[string]any)
								for prop := s.Properties.First(); prop != nil; prop = prop.Next() {
									name := prop.Key()
									if v, ok := req.Params.Arguments[name]; ok {
										bodyParams[name] = v
									}
								}
							}
						}
					}
				}

				if len(q) > 0 {
					u.RawQuery = q.Encode()
				}

				var reqBody io.Reader
				if len(bodyParams) > 0 {
					b, err := json.Marshal(bodyParams)
					if err != nil {
						return nil, fmt.Errorf("marshal body: %w", err)
					}
					reqBody = bytes.NewReader(b)
				}

				hreq, err := http.NewRequest(method, u.String(), reqBody)
				if err != nil {
					return nil, err
				}
				for k, vs := range headers {
					for _, v := range vs {
						hreq.Header.Add(k, v)
					}
				}
				if reqBody != nil {
					hreq.Header.Set("Content-Type", "application/json")
				}

				resp, err := client.Do(hreq)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, err
				}
				if resp.StatusCode >= 400 {
					return &mcp.CallToolResultFor[any]{
						Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Request failed with status %d: %s", resp.StatusCode, string(body))}},
						IsError: true,
					}, nil
				}
				ct := resp.Header.Get("Content-Type")
				var content mcp.Content
				switch {
				case strings.HasPrefix(ct, "image/"):
					content = &mcp.ImageContent{Data: body, MIMEType: ct}
				case strings.Contains(ct, "application/json"):
					var pretty bytes.Buffer
					if json.Indent(&pretty, body, "", "  ") == nil {
						body = pretty.Bytes()
					}
					content = &mcp.TextContent{Text: string(body)}
				default:
					content = &mcp.TextContent{Text: string(body)}
				}
				return &mcp.CallToolResultFor[any]{Content: []mcp.Content{content}}, nil
			})
		}
	}
	return nil
}

func addParamToSchema(schema *jsonschema.Schema, param *v3.Parameter) {
	if param == nil || param.Schema == nil {
		return
	}
	ps := &jsonschema.Schema{Type: typeOfSchema(param.Schema.Schema())}
	if s := param.Schema.Schema(); s != nil {
		ps.Description = buildSchemaDescription(param.Description, s)
		if s.Pattern != "" {
			ps.Pattern = s.Pattern
		}
	}
	schema.Properties[param.Name] = ps
	if param.Required != nil && *param.Required {
		schema.Required = append(schema.Required, param.Name)
	}
}

func typeOfSchema(s *base.Schema) string {
	if s == nil || len(s.Type) == 0 {
		return "string"
	}
	return s.Type[0]
}

func buildSchemaDescription(paramDesc string, paramSchema *base.Schema) string {
	description := paramDesc
	if paramSchema.Description != "" {
		if description != "" && description != paramSchema.Description {
			description = fmt.Sprintf("%s. %s", description, paramSchema.Description)
		} else {
			description = paramSchema.Description
		}
	}
	var enumValues []string
	if len(paramSchema.Enum) > 0 {
		enumValues = getEnumValues(paramSchema.Enum)
	}
	if len(enumValues) > 0 {
		if description != "" {
			description = fmt.Sprintf("%s (Allowed values: %s)", description, strings.Join(enumValues, ", "))
		} else {
			description = fmt.Sprintf("Allowed values: %s", strings.Join(enumValues, ", "))
		}
	}
	return description
}

func getEnumValues(enum []*yaml.Node) []string {
	if len(enum) == 0 {
		return nil
	}
	values := make([]string, len(enum))
	for i, v := range enum {
		values[i] = v.Value
	}
	return values
}

func getToolName(operationId string) string {
	if len(operationId) <= 64 {
		return operationId
	}
	hash := sha256.Sum256([]byte(operationId))
	shortHash := base64.RawURLEncoding.EncodeToString(hash[:])[:8]
	return operationId[:55] + "_" + shortHash
}

func applyParam(param *v3.Parameter, args map[string]any, u *url.URL, q url.Values, headers http.Header) {
	if param == nil {
		return
	}
	value, ok := args[param.Name]
	if !ok {
		return
	}
	switch param.In {
	case "path":
		val := fmt.Sprint(value)
		u.Path = strings.ReplaceAll(u.Path, "{"+param.Name+"}", pathSegmentEscape(val))
	case "query":
		switch v := value.(type) {
		case []any:
			strs := make([]string, len(v))
			for i, it := range v {
				strs[i] = fmt.Sprint(it)
			}
			q.Set(param.Name, strings.Join(strs, ","))
		default:
			q.Set(param.Name, fmt.Sprint(value))
		}
	case "header":
		headers.Add(param.Name, fmt.Sprint(value))
	}
}

// pathSegmentEscape preserves valid URL segment characters per RFC 3986.
func pathSegmentEscape(s string) string {
	hexCount := 0
	for i := 0; i < len(s); i++ {
		if shouldEscape(s[i]) {
			hexCount++
		}
	}
	if hexCount == 0 {
		return s
	}
	var buf [3]byte
	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			buf[0] = '%'
			buf[1] = "0123456789ABCDEF"[c>>4]
			buf[2] = "0123456789ABCDEF"[c&15]
			t[j] = buf[0]
			t[j+1] = buf[1]
			t[j+2] = buf[2]
			j += 3
		} else {
			t[j] = c
			j++
		}
	}
	return string(t)
}

func shouldEscape(c byte) bool {
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return false
	}
	switch c {
	case '-', '.', '_', '~':
		return false
	case '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=', ':', '@':
		return false
	}
	return true
}
