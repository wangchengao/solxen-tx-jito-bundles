package logic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"math/rand"
	"net/http"
	"time"
)

type JitoResponse struct {
	Result string
}

var jitoRand = rand.New(rand.NewSource(time.Now().UnixNano()))

var rpcEndpoints = []string{
	"https://mainnet.block-engine.jito.wtf/api/v1/bundles",
	"https://amsterdam.mainnet.block-engine.jito.wtf/api/v1/bundles",
	"https://frankfurt.mainnet.block-engine.jito.wtf/api/v1/bundles",
	"https://ny.mainnet.block-engine.jito.wtf/api/v1/bundles",
	"https://tokyo.mainnet.block-engine.jito.wtf/api/v1/bundles",
}

func getEndpoint() string {
	return rpcEndpoints[jitoRand.Intn(len(rpcEndpoints))]
}

func makeJitoRequest(method string, params interface{}) (string, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return "", fmt.Errorf("fail to encode request body: %v", err)
	}

	response, err := http.Post(getEndpoint(), "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("fail to send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		bodyText, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("status code: %d, response: %s", response.StatusCode, string(bodyText))
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("fail to read response content: %v", err)
	}

	var resp JitoResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		return "", fmt.Errorf("fail to deserialize response: %v, response: %s, status: %d", err, string(responseBody), response.StatusCode)
	}

	return resp.Result, nil
}

func sendBundle(bundleSignatures []string) (string, error) {
	// check tx limit
	if len(bundleSignatures) == 0 {
		return "", errors.New("empty bundle")
	}
	if len(bundleSignatures) > 5 {
		return "", errors.New("tx exceed limit(5)")
	}

	// 示例中没有明确如何处理Transaction编码，这里假设已经是可直接使用的形式
	result, err := makeJitoRequest("sendBundle", []interface{}{bundleSignatures})
	if err != nil {
		return "", err
	}

	return result, nil
}

type JitoTips struct {
	P25Landed float64 `json:"landed_tips_25th_percentile"`
	P50Landed float64 `json:"landed_tips_50th_percentile"`
	P75Landed float64 `json:"landed_tips_75th_percentile"`
	P95Landed float64 `json:"landed_tips_95th_percentile"`
	P99Landed float64 `json:"landed_tips_99th_percentile"`
}

var JitoRealTimeTips JitoTips

func (jt *JitoTips) String() string {
	return fmt.Sprintf(" p25=%d	p50=%d	p75=%d	p95=%d	p99=%d",
		int(jt.P25Landed*1e9), int(jt.P50Landed*1e9), int(jt.P75Landed*1e9), int(jt.P95Landed*1e9), int(jt.P99Landed*1e9))
}

func subscribeJitoTips(url string) {
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Printf("failed to connect to WebSocket: %v\n", err)
		return
	}
	defer c.Close()

	for {
		_, message, err := c.ReadMessage()
		logx.Infof("boundle tips msg: %v", string(message))
		if err != nil {
			fmt.Printf("read error: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		var tips []JitoTips
		if err = json.Unmarshal(message, &tips); err != nil {
			fmt.Printf("unmarshal error: %v\n", err)
			continue
		}

		if len(tips) > 0 {
			logx.Infof("jito tips: %v", tips[0].String())
			JitoRealTimeTips = tips[0]
		}
	}
}

func InitBundle() {
	go subscribeJitoTips("ws://bundles-api-rest.jito.wtf/api/v1/bundles/tip_stream")

	for true {
		logx.Infof("waiting init first tips")
		time.Sleep(1 * time.Second)
		if JitoRealTimeTips.P99Landed != 0 {
			break
		}
	}
}

var tipAddress = []solana.PublicKey{
	solana.MustPublicKeyFromBase58("HFqU5x63VTqvQss8hp11i4wVV8bD44PvwucfZ2bU7gRe"),
	solana.MustPublicKeyFromBase58("ADuUkR4vqLUMWXxW9gh6D6L8pMSawimctcNZ5pGwDcEt"),
	solana.MustPublicKeyFromBase58("ADaUMid9yfUytqMBgopwjb2DTLSokTSzL1zt6iGPaS49"),
	solana.MustPublicKeyFromBase58("DfXygSm4jCyNCybVYYK6DwvWqjKee8pbDmJGcLWNDXjh"),
	solana.MustPublicKeyFromBase58("3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6jT"),
	solana.MustPublicKeyFromBase58("Cw8CFyM9FkoMi7K7Crf6HNQqf4uEMzpKw6QNghXLvLkY"),
	solana.MustPublicKeyFromBase58("96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5"),
	solana.MustPublicKeyFromBase58("DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL"),
}

func GetTipAddress() solana.PublicKey {
	return tipAddress[jitoRand.Intn(len(tipAddress))]
}
