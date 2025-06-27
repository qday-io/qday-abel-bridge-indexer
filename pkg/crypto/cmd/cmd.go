package cmd

import (
	"encoding/hex"
	"fmt"
	"syscall"

	"github.com/qday-io/qday-abel-bridge-indexer/pkg/crypto"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func Crypto() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crypto",
		Short: "local crypto, Encrypt/Decrypt",
	}
	cmd.AddCommand(
		generateRsaKey(),
		rsaEncrypt(),
		rsaDecrypt(),
		aesEncrypt(),
		aesDecrypt(),
		genAesKey(),
		safeRsaEncrypt(),
		safeAesEncrypt(),
	)
	return cmd
}

func generateRsaKey() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gen-rsa-key",
		Short: "generate rsa PrivateKey",
		RunE: func(cmd *cobra.Command, _ []string) error {
			priv, pub, err := crypto.GenRsaKey(2048)
			if err != nil {
				return err
			}
			cmd.Println()
			cmd.Println("pubKey:\n", pub)
			cmd.Println("privateKey:\n", priv)
			return nil
		},
	}
	return cmd
}

func safeRsaEncrypt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "safe-rsa-enc",
		Short: "safe rsa encrypt hex, example: rsa-enc",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var (
				srcData string
				key     string
			)
			if term.IsTerminal(syscall.Stdin) {
				fmt.Print("Enter srcData: ")
				srcDataStdin, _ := term.ReadPassword(syscall.Stdin)
				fmt.Println()
				srcData = string(srcDataStdin)
				fmt.Print("Enter rsa publicKey: ")
				keyStdin, _ := term.ReadPassword(syscall.Stdin)
				fmt.Println()
				key = string(keyStdin)
			} else {
				return fmt.Errorf("error: Cannot read srcData and key from non-terminal input")
			}
			if srcData == "" || key == "" {
				return fmt.Errorf("invalid parameter")
			}
			data, err := crypto.RsaEncryptHex(srcData, key)
			if err != nil {
				return err
			}
			cmd.Println()
			cmd.Println("rsa enc data:\n", data)
			return nil
		},
	}
	return cmd
}

func rsaEncrypt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rsa-enc",
		Short: "rsa encrypt hex, example: rsa-enc {srcData} {key}",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("invalid parameter")
			}
			data, err := crypto.RsaEncryptHex(args[0], args[1])
			if err != nil {
				return err
			}
			cmd.Println()
			cmd.Println("rsa enc data:\n", data)
			return nil
		},
	}
	return cmd
}

func rsaDecrypt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rsa-dec",
		Short: "rsa decrypt hex, example: src-dec {encryptedData} {rsaPrivateKey}",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("invalid parameter")
			}
			data, err := crypto.RsaDecryptHex(args[0], args[1])
			if err != nil {
				return err
			}
			cmd.Println("rsa dec data:\n", data)
			return nil
		},
	}
	return cmd
}

func aesEncrypt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aes-enc",
		Short: "aes256 encrypt hex, example: aes-enc {srcData} {key}",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("invalid parameter")
			}
			key, err := hex.DecodeString(args[1])
			if err != nil {
				return err
			}
			data, err := crypto.AesEncrypt([]byte(args[0]), key)
			if err != nil {
				return err
			}
			cmd.Println("aes enc data:\n", hex.EncodeToString(data))
			return nil
		},
	}
	return cmd
}

func safeAesEncrypt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "safe-aes-enc",
		Short: "safe aes256 encrypt hex, example: aes-enc",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var (
				srcData string
				srcKey  string
			)
			if term.IsTerminal(syscall.Stdin) {
				fmt.Print("Enter srcData: ")
				srcDataStdin, _ := term.ReadPassword(syscall.Stdin)
				fmt.Println()
				srcData = string(srcDataStdin)
				fmt.Print("Enter key: ")
				keyStdin, _ := term.ReadPassword(syscall.Stdin)
				fmt.Println()
				srcKey = string(keyStdin)
			} else {
				return fmt.Errorf("error: Cannot read srcData and key from non-terminal input")
			}
			if srcData == "" || srcKey == "" {
				return fmt.Errorf("invalid parameter")
			}
			key, err := hex.DecodeString(srcKey)
			if err != nil {
				return err
			}
			data, err := crypto.AesEncrypt([]byte(srcData), key)
			if err != nil {
				return err
			}
			cmd.Println("aes enc data:\n", hex.EncodeToString(data))
			return nil
		},
	}
	return cmd
}

func aesDecrypt() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aes-dec",
		Short: "aes256 decrypt hex, example: aes-dec {cryptedData}",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("invalid parameter")
			}
			var key string
			if term.IsTerminal(syscall.Stdin) {
				fmt.Print("Enter key: ")
				keyStdin, _ := term.ReadPassword(syscall.Stdin)
				fmt.Println()
				key = string(keyStdin)
			} else {
				return fmt.Errorf("error: Cannot read key from non-terminal input")
			}
			if key == "" {
				return fmt.Errorf("invalid key")
			}
			srcData, err := hex.DecodeString(args[0])
			if err != nil {
				return err
			}
			keyByte, err := hex.DecodeString(key)
			if err != nil {
				return err
			}
			data, err := crypto.AesDecrypt(srcData, keyByte)
			if err != nil {
				return err
			}
			cmd.Println("aes dec data:\n", string(data))
			return nil
		},
	}
	return cmd
}

func genAesKey() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gen-aes-key",
		Short: "generate aes256 key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := crypto.GenAesKey()
			if err != nil {
				return err
			}
			cmd.Println("aes key:\n", hex.EncodeToString(key))
			return nil
		},
	}
	return cmd
}
