package config

import _ "embed"

// DefaultConfigJSON 编译嵌入的默认配置文件，install 时写入 exe 目录
//
//go:embed default_config.json
var DefaultConfigJSON []byte
