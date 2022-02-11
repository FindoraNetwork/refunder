package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ReadConfig(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		want    *config
		wantErr bool
	}{
		{
			name: "happy case",
			setup: func() {
				os.Setenv(EnvServerAddress, "EnvServerAddress")
				os.Setenv(EnvMaxRefund, "100")
				os.Setenv(EnvBridgeTokenAddress, "EnvBridgeTokenAddress")
			},
			want: &config{
				ServerAddr:      "EnvServerAddress",
				MaxRefund:       100,
				BridgeTokenAddr: "EnvBridgeTokenAddress",
			},
		},
		{
			name: "maxRefund parsing failed",
			setup: func() {
				os.Setenv(EnvServerAddress, "EnvServerAddress")
				os.Setenv(EnvMaxRefund, "not-parsable")
				os.Setenv(EnvBridgeTokenAddress, "EnvBridgeTokenAddress")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got, gotErr := readConfig()
			if tt.wantErr {
				assert.Nil(t, got)
				assert.Error(t, gotErr)
			} else {
				assert.Equal(t, got, tt.want)
				assert.NoError(t, gotErr)
			}
		})

	}
}
