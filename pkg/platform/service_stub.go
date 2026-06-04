//go:build !windows

package platform

import "errors"

var errNotSupported = errors.New("当前平台不支持此操作，请使用 hooks 方式管理 server 生命周期")

func StartupDir() string                   { return "" }
func GenerateVBS() (string, error)         { return "", errNotSupported }
func GenerateConfig() (string, error)      { return "", errNotSupported }
func InstallStartup() error                { return errNotSupported }
func UninstallStartup() error              { return errNotSupported }
func StopServer() error                    { return errNotSupported }
func QueryStatus() (bool, int, error)      { return false, 0, errNotSupported }
