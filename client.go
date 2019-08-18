package client

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/machinebox/graphql"
)

// ChatPlugClient holds connection with chatplug core server
type ChatPlugClient struct {
	GQLClient             *GQLClient
	MessagesChan          chan *MessageReceived
	ConfigurationRecvChan chan *ConfigurationResponse
	SearchRequestsChan    chan *SearchRequest
	msgSubID              string
	cfgSubID              string
	searchSubID           string
}

func NewChatPlugClient(wsURL string, httpUrl string, accessToken string) *ChatPlugClient {
	return &ChatPlugClient{
		GQLClient:             NewGQLClient(wsURL, httpUrl, PayloadMessage{AccessToken: accessToken}),
		MessagesChan:          make(chan *MessageReceived),
		ConfigurationRecvChan: make(chan *ConfigurationResponse),
		SearchRequestsChan:    make(chan *SearchRequest),
		msgSubID:              "",
		cfgSubID:              "",
		searchSubID:           "",
	}
}

func (cpc *ChatPlugClient) Close() {
	_ = cpc.GQLClient.wsConn.Close()
}

// SendMessage sends a message with given data to core server via graphql
func (cpc *ChatPlugClient) SendMessage(body string, originId string, originThreadId string, username string, authorOriginId string, authorAvatarUrl string, attachments []*AttachmentInput) {
	req := graphql.NewRequest(sendMessageMutation)
	req.Var("body", body)
	req.Var("originId", originId)
	req.Var("originThreadId", originThreadId)
	req.Var("username", username)
	req.Var("authorOriginId", authorOriginId)
	req.Var("authorAvatarUrl", authorAvatarUrl)
	req.Var("attachments", attachments)

	fmt.Println("Sending sendMessage mutation to the core")
	_, err := cpc.GQLClient.Request(req)
	if err != nil {
		fmt.Println("error occured")
		fmt.Println(err)
	}
}

// SetSearchResponse sets a response to a given search query
func (cpc *ChatPlugClient) SetSearchResponse(forQuery string, threads []*SearchThreadInput) {
	req := graphql.NewRequest(setSearchResultMutation)
	req.Var("q", forQuery)
	req.Var("res", threads)

	fmt.Println("Sending sendMessage mutation to the core")
	_, err := cpc.GQLClient.Request(req)
	if err != nil {
		fmt.Println("error occured")
		fmt.Println(err)
	}
}

// SubscribeToNewMessages starts a subscription to core server's messages
func (cpc *ChatPlugClient) SubscribeToNewMessages() {
	cpc.msgSubID = cpc.GQLClient.Subscribe(messageReceivedSubscription, map[string]interface{}{})
}

// SubscribeToNewMessages starts a subscription to thread search requests
func (cpc *ChatPlugClient) SubscribeToSearchRequests() {
	cpc.searchSubID = cpc.GQLClient.Subscribe(searchRequestSubscription, map[string]interface{}{})
}

func (cpc *ChatPlugClient) Connect() {
	packets, _ := cpc.GQLClient.Connect()

	go func() {
		for packet := range packets {
			log.Println(packet.Type)
			log.Println(packet.ID)
			log.Println(cpc.cfgSubID)

			if packet.Type == "data" {
				if packet.ID == cpc.msgSubID {
					var msg messageReceivedPayload
					err := json.Unmarshal(*packet.Payload, &msg)
					if err != nil {
						fmt.Printf(err.Error())
					}
					cpc.MessagesChan <- &msg.Data.MessageReceived
				}
				if packet.ID == cpc.cfgSubID {
					var cfg configurationReceivedPayload
					err := json.Unmarshal(*packet.Payload, &cfg)
					if err != nil {
						fmt.Printf(err.Error())
					}
					cpc.ConfigurationRecvChan <- &cfg.Data.ConfigurationReceived
				}
				if packet.ID == cpc.searchSubID {
					var req searchRequestPayload
					err := json.Unmarshal(*packet.Payload, &req)
					if err != nil {
						fmt.Printf(err.Error())
					}
					cpc.SearchRequestsChan <- &req.Data.SubscribeToSearchRequests
				}
			}
		}
	}()
}

func (cpc *ChatPlugClient) SubscribeToConfigResponses(configurationSchema []ConfigurationField) {
	variables := make(map[string]interface{})
	variables["fields"] = configurationSchema
	cpc.cfgSubID = cpc.GQLClient.Subscribe(requestConfigurationRequest, variables)
}
