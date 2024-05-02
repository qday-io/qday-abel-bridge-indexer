package aa

import (
	"encoding/json"
	"fmt"

	"github.com/b2network/b2-indexer/pkg/log"
	"github.com/b2network/b2-indexer/pkg/rpc"
	"github.com/tidwall/gjson"
)

var AddressNotFoundErrCode = "1001"

type Response struct {
	Code    string
	Message string
	Data    struct {
		Pubkey string
	}
}

func GetPubKey(api, btcAddress string, network string) (*Response, error) {
	uri := fmt.Sprintf("%v/api/bridge/toAddress?from_network=%v&from_address=%v", api, network, btcAddress)
	res, err := rpc.HTTPGet(uri)
	if err != nil {
		return nil, err
	}

	log.Infof("get pubkey response:%v", string(res))

	root := gjson.ParseBytes(res)

	code := root.Get("code").Int()
	msg := root.Get("message").String()
	pubKey := root.Get("data.to_address").String()

	if code != 0 || len(pubKey) < 1 {
		log.Warnf("not found L2 address for btcAddres:%v \n", btcAddress)
		return nil, fmt.Errorf("not found L2 address for btcAddres:%v", btcAddress)
	}

	btcResp := Response{Code: fmt.Sprintf("%v", code), Message: msg, Data: struct{ Pubkey string }{Pubkey: pubKey}}

	err = json.Unmarshal(res, &btcResp)
	if err != nil {
		return nil, err
	}

	return &btcResp, nil

	//return &Response{Code: fmt.Sprintf("%v", 0), Message: "0k", Data: struct{ Pubkey string }{Pubkey: "0x002E73CaaBD414eeaFE0fe3ecA18F4c7D9069207"}}, nil
}
