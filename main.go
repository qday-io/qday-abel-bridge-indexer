/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"github.com/qday-io/qday-abel-bridge-indexer/cmd"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // 自动加载 .env 文件
	cmd.Execute()
}
