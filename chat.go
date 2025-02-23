package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type ContentType string

// Chat message role defined by the OpenAI API.
const (
	ChatMessageRoleSystem                = "system"
	ChatMessageRoleUser                  = "user"
	ChatMessageRoleAssistant             = "assistant"
	ChatMessageRoleFunction              = "function"
	ChatMessageRoleTool                  = "tool"
	ContentTypeText          ContentType = "text"
	ContentTypeImage         ContentType = "image_url"
)

const chatCompletionsSuffix = "/chat/completions"

var (
	ErrChatCompletionInvalidModel       = errors.New("this model is not supported with this method, please use CreateCompletion client method instead") //nolint:lll
	ErrChatCompletionStreamNotSupported = errors.New("streaming is not supported with this method, please use CreateChatCompletionStream")              //nolint:lll
)

type Hate struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type SelfHarm struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type Sexual struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type Violence struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}

type ContentFilterResults struct {
	Hate     Hate     `json:"hate,omitempty"`
	SelfHarm SelfHarm `json:"self_harm,omitempty"`
	Sexual   Sexual   `json:"sexual,omitempty"`
	Violence Violence `json:"violence,omitempty"`
}

type PromptAnnotation struct {
	PromptIndex          int                  `json:"prompt_index,omitempty"`
	ContentFilterResults ContentFilterResults `json:"content_filter_results,omitempty"`
}

type Part struct {
	Type     ContentType `json:"type"`
	ImageUrl string      `json:"image_url,omitempty"`
	Text     string      `json:"text,omitempty"`
}

type Parts []Part

func (ps Parts) MarshalJSON() ([]byte, error) {
	if len(ps) == 0 {
		return []byte("\"\""), nil
	}
	cc := []Part(ps)
	if len(cc) == 1 && cc[0].Type == "text" {
		if cc[0].Text != "" {
			return json.Marshal(cc[0].Text)
		}
		return []byte("\"\""), nil
	}
	return json.Marshal([]Part(cc))
}

func (ps *Parts) UnmarshalJSON(bs []byte) error {
	if bs[0] == '"' && bs[len(bs)-1] == '"' {
		if len(bs) == 2 {
			*ps = nil
			return nil
		}
		var s string
		err := json.Unmarshal(bs, &s)
		if err != nil {
			return err
		}
		*ps = Parts{{Type: ContentTypeText, Text: s}}
		return nil
	}
	var parts []Part
	err := json.Unmarshal(bs, &parts)
	if err != nil {
		return err
	}
	*ps = parts
	return nil
}

type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Parts   Parts  `json:"content"`

	// This property isn't in the official documentation, but it's in
	// the documentation for the official library for python:
	// - https://github.com/openai/openai-python/blob/main/chatml.md
	// - https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
	Name string `json:"name,omitempty"`

	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}

func (m *ChatCompletionMessage) UnmarshalJSON(bs []byte) error {
	msg := struct {
		Role         string        `json:"role"`
		Content      string        `json:"-"`
		Parts        Parts         `json:"content"`
		Name         string        `json:"name,omitempty"`
		FunctionCall *FunctionCall `json:"function_call,omitempty"`
		ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	}(*m)
	err := json.Unmarshal(bs, &msg)
	if err != nil {
		return err
	}
	*m = ChatCompletionMessage(msg)
	if len(m.Parts) == 1 && m.Parts[0].Type == ContentTypeText {
		m.Content = m.Parts[0].Text
	}
	return nil
}

func (m ChatCompletionMessage) MarshalJSON() ([]byte, error) {
	msg := struct {
		Role         string        `json:"role"`
		Content      string        `json:"-"`
		Parts        Parts         `json:"content"`
		Name         string        `json:"name,omitempty"`
		FunctionCall *FunctionCall `json:"function_call,omitempty"`
		ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	}(m)
	if msg.Content != "" && len(msg.Parts) == 1 && msg.Parts[0].Type == ContentTypeText && msg.Parts[0].Text == msg.Content {
	} else if msg.Content != "" && len(msg.Parts) > 0 {
		return nil, fmt.Errorf("Content and Parts are mutually exclusive")
	} else if msg.Content != "" {
		msg.Parts = Parts{{Type: ContentTypeText, Text: msg.Content}}
	}
	return json.Marshal(msg)
}

type ToolCall struct {
	ID       string       `json:"id"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name string `json:"name,omitempty"`
	// call function with arguments in JSON format
	Arguments string `json:"arguments,omitempty"`
}

type ChatCompletionResponseFormatType string

const (
	ChatCompletionResponseFormatTypeJSONObject ChatCompletionResponseFormatType = "json_object"
	ChatCompletionResponseFormatTypeText       ChatCompletionResponseFormatType = "text"
)

type ChatCompletionResponseFormat struct {
	Type ChatCompletionResponseFormatType `json:"type"`
}

// ChatCompletionRequest represents a request structure for chat completion API.
type ChatCompletionRequest struct {
	Model            string                        `json:"model"`
	Messages         []ChatCompletionMessage       `json:"messages"`
	MaxTokens        int                           `json:"max_tokens,omitempty"`
	Temperature      float32                       `json:"temperature,omitempty"`
	TopP             float32                       `json:"top_p,omitempty"`
	N                int                           `json:"n,omitempty"`
	Stream           bool                          `json:"stream,omitempty"`
	Stop             []string                      `json:"stop,omitempty"`
	PresencePenalty  float32                       `json:"presence_penalty,omitempty"`
	ResponseFormat   *ChatCompletionResponseFormat `json:"response_format,omitempty"`
	Seed             *int                          `json:"seed,omitempty"`
	FrequencyPenalty float32                       `json:"frequency_penalty,omitempty"`
	// LogitBias is must be a token id string (specified by their token ID in the tokenizer), not a word string.
	// incorrect: `"logit_bias":{"You": 6}`, correct: `"logit_bias":{"1639": 6}`
	// refs: https://platform.openai.com/docs/api-reference/chat/create#chat/create-logit_bias
	LogitBias map[string]int `json:"logit_bias,omitempty"`
	User      string         `json:"user,omitempty"`
	// Deprecated: use Tools instead.
	Functions []FunctionDefinition `json:"functions,omitempty"`
	// Deprecated: use ToolChoice instead.
	FunctionCall any    `json:"function_call,omitempty"`
	Tools        []Tool `json:"tools,omitempty"`
	// This can be either a string or an ToolChoice object.
	ToolChoiche any `json:"tool_choice,omitempty"`
}

type ToolType string

const (
	ToolTypeFunction ToolType = "function"
)

type Tool struct {
	Type     ToolType           `json:"type"`
	Function FunctionDefinition `json:"function,omitempty"`
}

type ToolChoiche struct {
	Type     ToolType     `json:"type"`
	Function ToolFunction `json:"function,omitempty"`
}

type ToolFunction struct {
	Name string `json:"name"`
}

type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Parameters is an object describing the function.
	// You can pass json.RawMessage to describe the schema,
	// or you can pass in a struct which serializes to the proper JSON schema.
	// The jsonschema package is provided for convenience, but you should
	// consider another specialized library if you require more complex schemas.
	Parameters any `json:"parameters"`
}

// Deprecated: use FunctionDefinition instead.
type FunctionDefine = FunctionDefinition

type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonFunctionCall  FinishReason = "function_call"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonNull          FinishReason = "null"
)

func (r FinishReason) MarshalJSON() ([]byte, error) {
	if r == FinishReasonNull || r == "" {
		return []byte("null"), nil
	}
	return []byte(`"` + string(r) + `"`), nil // best effort to not break future API changes
}

type ChatCompletionChoice struct {
	Index   int                   `json:"index"`
	Message ChatCompletionMessage `json:"message"`
	// FinishReason
	// stop: API returned complete message,
	// or a message terminated by one of the stop sequences provided via the stop parameter
	// length: Incomplete model output due to max_tokens parameter or token limit
	// function_call: The model decided to call a function
	// content_filter: Omitted content due to a flag from our content filters
	// null: API response still in progress or incomplete
	FinishReason FinishReason `json:"finish_reason"`
}

// ChatCompletionResponse represents a response structure for chat completion API.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   Usage                  `json:"usage"`

	httpHeader
}

// CreateChatCompletion — API call to Create a completion for the chat message.
func (c *Client) CreateChatCompletion(
	ctx context.Context,
	request ChatCompletionRequest,
) (response ChatCompletionResponse, err error) {
	if request.Stream {
		err = ErrChatCompletionStreamNotSupported
		return
	}

	urlSuffix := chatCompletionsSuffix
	if !checkEndpointSupportsModel(urlSuffix, request.Model) {
		err = ErrChatCompletionInvalidModel
		return
	}

	req, err := c.newRequest(ctx, http.MethodPost, c.fullURL(urlSuffix, request.Model), withBody(request))
	if err != nil {
		return
	}

	err = c.sendRequest(req, &response)
	return
}
