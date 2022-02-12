package config_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/FindoraNetwork/refunder/config"

	"github.com/stretchr/testify/assert"
)

func Test_ReadConfig(t *testing.T) {
	tests := []struct {
		name         string
		cmd          string
		filepath     string
		conf_content string
		want         *config.Config
		wantErr      bool
	}{
		{
			name:         "happy case",
			cmd:          "--config",
			conf_content: "{}",
			want:         &config.Config{},
		},
		{
			name:    "cmd not as expect",
			cmd:     "--something_else",
			wantErr: true,
		},
		{
			name:     "read config file failed",
			cmd:      "--config",
			filepath: "not-exists-filepath",
			wantErr:  true,
		},
		{
			name:         "parsing json failed",
			cmd:          "--config",
			conf_content: "{-----}",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fname := strings.Join(strings.Split(tt.name, " "), "_")

			f, err := ioutil.TempFile("", "Test_ReadConfig_"+fname+".*.json")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(f.Name())

			if _, err := f.WriteString(tt.conf_content); err != nil {
				t.Fatal(err)
			}

			fpath := f.Name()
			if tt.filepath != "" {
				fpath = tt.filepath
			}

			got, gotErr := config.Load(tt.cmd, fpath)
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
