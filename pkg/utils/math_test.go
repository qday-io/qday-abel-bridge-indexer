package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertScientificToBigIntString(t *testing.T) {
	s, err := ConvertScientificToBigIntString("2e10")
	assert.NoError(t, err)
	t.Log(s)
}
