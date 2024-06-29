package aa

import (
	"fmt"
	"io"
	"net/http"

	"github.com/b2network/b2-indexer/pkg/log"
	"github.com/tidwall/gjson"
)

var AddressNotFoundErrCode = "1001"

type Response struct {
	Code      string
	Message   string
	AAAddress string
}

func GetPubKey(api, txId, btcFromAddress string, btcFromNetwork string) (*Response, error) {
	uri := fmt.Sprintf("%v/api/bridge/hash?hash=%v", api, txId)

	res, err := http.Get(uri) //nolint
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StatusCode: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	log.Infof("Get Pubkey response:%v", string(body))

	root := gjson.ParseBytes(body)

	code := root.Get("code").Int()
	msg := root.Get("message").String()
	pubKey := root.Get("data.to_address").String()
	fromNet := root.Get("data.from_network").String()
	fromAddr := root.Get("data.from_address").String()

	if fromNet != btcFromNetwork || fromAddr != btcFromAddress {
		log.Warnf("bridge data error: hash:%v,btcFromNetwork:%v,btcFromAddress:%v,but onchain data: fromNetwork:%v,fromAddress:%v", txId, btcFromNetwork, btcFromAddress, fromNet, fromAddr)
		return nil, fmt.Errorf("bridge data error: hash:%v,btcFromNetwork:%v,btcFromAddress:%v,but onchain data: fromNetwork:%v,fromAddress:%v", txId, btcFromNetwork, btcFromAddress, fromNet, fromAddr)
	}

	if code != 0 || len(pubKey) < 1 {
		log.Warnf("not found L2 address for btcAddres:%v \n", btcFromAddress)
		return nil, fmt.Errorf("not found L2 address for btcAddres:%v", btcFromAddress)
	}

	btcResp := Response{Code: fmt.Sprintf("%v", code), Message: msg, AAAddress: pubKey}
	return &btcResp, nil
}
