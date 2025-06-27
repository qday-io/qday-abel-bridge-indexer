package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_buildIndexCmd(t *testing.T) {
	// 测试命令构建，不执行实际启动
	cmd := buildIndexCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "start", cmd.Name())
}
