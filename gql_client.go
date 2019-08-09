package client

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/machinebox/graphql"
)

const (
	connectionInitMsg   = "connection_init" // Client -> Server
	startMsg            = "start"           // Client -> Server
	sendMessageMutation = `
	mutation sendMessage($body: String!, $originId: String!, $originThreadId: String!, $username: String!, $authorOriginId: String!, $authorAvatarUrl: String!, $attachments: [AttachmentInput!]!) {
		sendMessage(
		  input: {
			body: $body,
			originId: $originId,
			originThreadId: $originThreadId,
			author: {
			  username: $username, 
			  originId: $authorOriginId,
			  avatarUrl: $authorAvatarUrl
			},
			attachments: $attachments
		  }
		) {
		  id
		}
	  }`
	messageReceivedSubscription = `
	  subscription {
		  messageReceived {
			message {
			  body
			  id
			  originId
			  attachments {
				  type
				  sourceUrl
				  originId
				  id
			  }
			  thread {
				  id
				  originId
				  name
			  }
			  threadGroupId
			  author {
				  username
				  originId
				  avatarUrl
			  }
			}
			targetThreadId
		  }
		}`
	setSearchResultMutation = `
	mutation setSearchResponse($q: String!, $res: [ThreadSearchResultInput!]!) {
		setSearchResponse(forQuery:$q, threads:$res) {
		  threads {
			originId
			name
			iconUrl
		  }
		  forQuery
		}
	  }`
	searchRequestSubscription = `
	  subscription {
		  subscribeToSearchRequests {
			query
		  }
		}`
	requestConfigurationRequest = `
	subscription confRequest($fields: [ConfigurationField!]!){
		configurationReceived(configuration:{fields: $fields}) {
		  fieldValues
		}
	  }`

	setInstanceStatusMutation = `
	mutation {
		setInstanceStatus(status:INITIALIZED) {
		  status
		  name
		}
	  }`
)

// MessageAuthor holds information about single message's atuhor
type MessageAuthor struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	OriginID  string `json:"originId"`
	AvatarURL string `json:"avatarUrl"`
}

type AttachmentInput struct {
	OriginID  string `json:"originId"`
	Type      string `json:"type"`
	SourceURL string `json:"sourceUrl"`
}

type Attachment struct {
	ID        string `json:"id"`
	OriginID  string `json:"originId"`
	Type      string `json:"type"`
	SourceURL string `json:"sourceUrl"`
}

// Thread holds information about single thread
type Thread struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	OriginID          string `json:"originId"`
	ThreadGroupID     string `json:"threadGroupId"`
	IconURL           string `json:"iconUrl"`
	ServiceInstanceID string `json:"serviceInstanceId"`
}

// Message holds information about single message
type Message struct {
	ID            string        `json:"string"`
	OriginID      string        `json:"originId"`
	Author        MessageAuthor `json:"author"`
	Thread        Thread        `json:"thread"`
	Body          string        `json:"body"`
	ThreadGroupID string        `json:"threadGroupId"`
	Attachments   []Attachment  `json:"attachments"`
}

type SearchThreadInput struct {
	OriginID string `json:"originId"`
	Name     string `json:"name"`
	IconURL  string `json:"iconUrl"`
}

// ErrorLocation holds data about location of gql error
type ErrorLocation struct {
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

// ErrorMessage holds data about graphql error
type ErrorMessage struct {
	Message   string           `json:"message,omitempty"`
	Locations []*ErrorLocation `json:"locations,omitempty"`
}

// MessageReceived holds data about incoming message
type MessageReceived struct {
	Message        Message `json:"message"`
	TargetThreadID string  `json:"targetThreadId"`
}

type SearchRequest struct {
	Query string `json:"query"`
}

type searchRequestPayload struct {
	Data struct {
		SubscribeToSearchRequests SearchRequest `json:"subscribeToSearchRequests"`
	} `json:"data"`
}

type messageReceivedPayload struct {
	Data struct {
		MessageReceived MessageReceived `json:"messageReceived"`
	} `json:"data"`
}

type configurationReceivedPayload struct {
	Data struct {
		ConfigurationReceived ConfigurationResponse `json:"configurationReceived"`
	} `json:"data"`
}

type OperationMessage struct {
	Payload PayloadMessage `json:"payload,omitempty"`
	ID      string         `json:"id,omitempty"`
	Type    string         `json:"type"`
}

// IncomingPayload is a struct holding graphql operation data
type IncomingPayload struct {
	Payload *json.RawMessage `json:"payload,omitempty"`
	Type    string           `json:"type"`
	ID      string           `json:"id"`
}

type PayloadMessage struct {
	Query       string                 `json:"query"`
	Variables   map[string]interface{} `json:"variables"`
	AccessToken string                 `json:"accessToken"`
}

type GQLClient struct {
	WSUrl   string
	HTTPUrl string
	Headers PayloadMessage
	client  *graphql.Client

	wsConn *websocket.Conn
}

func NewGQLClient(wsUrl string, httpUrl string, headers PayloadMessage) *GQLClient {
	return &GQLClient{
		WSUrl:   wsUrl,
		HTTPUrl: httpUrl,
		Headers: headers,
		client:  graphql.NewClient(httpUrl),
	}
}

// Request sends a graphql requests to the core server and returns a pointer to map with result
func (gqc *GQLClient) Request(req *graphql.Request) (*map[string]interface{}, error) {
	// make a request
	req.Header.Add("Authentication", gqc.Headers.AccessToken)
	ctx := context.Background()
	var respData map[string]interface{}

	if err := gqc.client.Run(ctx, req, &respData); err != nil {
		return nil, err
	}
	return &respData, nil
}

func (gqc *GQLClient) ReadIncomingPayload() (*IncomingPayload, error) {
	var msg IncomingPayload
	err := gqc.wsConn.ReadJSON(&msg)
	if err != nil {
		panic(err)
	}
	return &msg, err
}

func (gqc *GQLClient) WriteOperationPacket(packet *OperationMessage) {
	_ = gqc.wsConn.WriteJSON(packet)
}

func (gqc *GQLClient) Subscribe(query string, variables map[string]interface{}) string {
	subID := GenerateID()
	gqc.WriteOperationPacket(&OperationMessage{Type: startMsg, ID: subID, Payload: PayloadMessage{
		Query:     query,
		Variables: variables,
	}})

	return subID
}

func (gqc *GQLClient) Connect() (<-chan *IncomingPayload, <-chan error) {
	headers := make(http.Header)
	headers.Add("Sec-Websocket-Protocol", "graphql-ws")
	ws, _, err := websocket.DefaultDialer.Dial(gqc.WSUrl, headers)

	if err != nil {
		panic(err)
	}
	gqc.wsConn = ws
	channel := make(chan *IncomingPayload)
	errChan := make(chan error)

	gqc.WriteOperationPacket(&OperationMessage{Type: connectionInitMsg, Payload: gqc.Headers})
	_, _ = gqc.ReadIncomingPayload()

	go func() {
		defer close(channel)
		defer close(errChan)
		for {
			msg, err := gqc.ReadIncomingPayload()
			if err != nil {
				errChan <- err
			}
			fmt.Println("got smth")
			if msg.Type == "error" {
				var errs []*ErrorMessage
				err = json.Unmarshal(*msg.Payload, &errs)
				fmt.Println(errs[0].Message)
			}
			fmt.Println(msg.ID)

			channel <- msg

		}
	}()

	return channel, errChan
}

type ConfigurationField struct {
	Type         string `json:"type"`
	DefaultValue string `json:"defaultValue"`
	Optional     bool   `json:"optional"`
	Hint         string `json:"hint"`
	Mask         bool   `json:"mask"`
}

type ConfigurationRequest struct {
	Fields []ConfigurationField `json:"fields"`
}

type ConfigurationResponse struct {
	FieldValues []string `json:"fieldValues"`
}

func GenerateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
