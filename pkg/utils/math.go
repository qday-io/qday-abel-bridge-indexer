package utils

import (
	"fmt"
	"math/big"
	"regexp"
)

func ConvertScientificToBigIntString(s string) (string, error) {
	// 正则表达式匹配科学计数法
	re := regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)[eE][+-]?\d+$`)
	if !re.MatchString(s) {
		return s, nil // 如果不是科学计数法，直接返回原字符串
	}

	// 使用 big.Float 解析科学计数法字符串
	bigFloat := new(big.Float)
	_, ok := bigFloat.SetString(s)
	if !ok {
		return "", fmt.Errorf("decode str error : %s", s)
	}

	// 转换为 big.Int
	bigInt := new(big.Int)
	bigFloat.Int(bigInt) // 提取整数部分

	// 返回整数的字符串表示
	return bigInt.String(), nil
}
