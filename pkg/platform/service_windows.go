//go:build windows

package platform

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"text/template"
	"time"
	"unsafe"

	"github.com/panda/qwen-usage/internal/config"
)

// VBS 脚本模板：隐藏窗口启动 server
var vbsTmpl = template.Must(template.New("vbs").Parse(`Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """{{.ExePath}}"" server", 0, False
`))

// StartupDir 返回 Windows 开机启动目录（用户目录下）
func StartupDir() string {
	return filepath.Join(os.Getenv("APPDATA"),
		"Microsoft", "Windows", "Start Menu", "Programs", "Startup")
}

// GenerateVBS 在 exe 目录生成 autostart.vbs，幂等
func GenerateVBS() (string, error) {
	exeDir, err := config.ExeDir()
	if err != nil {
		return "", err
	}
	vbsPath := filepath.Join(exeDir, "autostart.vbs")

	exePath, _ := os.Executable()
	exePath, _ = filepath.Abs(exePath)

	var buf bytes.Buffer
	if err := vbsTmpl.Execute(&buf, struct{ ExePath string }{exePath}); err != nil {
		return "", fmt.Errorf("渲染 VBS 模板失败: %w", err)
	}
	content := buf.Bytes()

	// 已存在且内容相同则跳过
	if existing, err := os.ReadFile(vbsPath); err == nil && bytes.Equal(existing, content) {
		return vbsPath, nil
	}

	if err := os.WriteFile(vbsPath, content, 0644); err != nil {
		return "", fmt.Errorf("写入启动脚本失败: %w", err)
	}
	return vbsPath, nil
}

// GenerateConfig 在 exe 目录生成 config.json，幂等
func GenerateConfig() (string, error) {
	exeDir, err := config.ExeDir()
	if err != nil {
		return "", err
	}
	cfgPath := filepath.Join(exeDir, "config.json")

	if _, err := os.Stat(cfgPath); err == nil {
		return cfgPath, nil
	}

	if err := os.WriteFile(cfgPath, config.DefaultConfigJSON, 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %w", err)
	}
	return cfgPath, nil
}

// InstallStartup 在 Startup 目录创建指向 autostart.vbs 的 .lnk 快捷方式，幂等
func InstallStartup() error {
	exeDir, err := config.ExeDir()
	if err != nil {
		return err
	}
	vbsPath := filepath.Join(exeDir, "autostart.vbs")
	shortcutPath := filepath.Join(StartupDir(), "qwen-usage.lnk")

	if _, err := os.Stat(shortcutPath); err == nil {
		return nil
	}

	return createShortcut(
		shortcutPath,
		"wscript.exe",
		fmt.Sprintf(`"%s"`, vbsPath),
		exeDir,
		0, // SW_HIDE
	)
}

// UninstallStartup 移除 Startup 目录的 .lnk 快捷方式，幂等
func UninstallStartup() error {
	shortcutPath := filepath.Join(StartupDir(), "qwen-usage.lnk")

	if _, err := os.Stat(shortcutPath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(shortcutPath)
}

// StopServer 优雅停止 server 并清理 PID 文件，幂等
func StopServer() error {
	cfg := config.GetConfig()
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://%s/health", cfg.ServerAddr)
	shutdownURL := fmt.Sprintf("http://%s/shutdown", cfg.ServerAddr)

	resp, err := client.Post(shutdownURL, "application/json", nil)
	if err != nil {
		if _, err2 := client.Get(healthURL); err2 != nil {
			RemovePIDFile()
			return nil
		}
		return fmt.Errorf("发送 shutdown 请求失败: %w", err)
	}
	_ = resp.Body.Close()

	for i := 0; i < 25; i++ {
		time.Sleep(200 * time.Millisecond)
		if _, err := client.Get(healthURL); err != nil {
			RemovePIDFile()
			return nil
		}
	}

	RemovePIDFile()
	return nil
}

// QueryStatus 查询 server 状态，返回 (running, pid, error)
func QueryStatus() (bool, int, error) {
	cfg := config.GetConfig()
	client := &http.Client{Timeout: 1 * time.Second}

	resp, err := client.Get(fmt.Sprintf("http://%s/health", cfg.ServerAddr))
	if err != nil {
		return false, 0, nil
	}
	_ = resp.Body.Close()

	pid, _ := ReadPIDFile()
	return true, pid, nil
}

// ── COM API 创建 .lnk 快捷方式 ──

var (
	ole32 = syscall.MustLoadDLL("ole32.dll")

	procCoInitializeEx  = ole32.MustFindProc("CoInitializeEx")
	procCoCreateInstance = ole32.MustFindProc("CoCreateInstance")
	procCoUninitialize  = ole32.MustFindProc("CoUninitialize")
)

func parseGUID(s string) (syscall.GUID, error) {
	if len(s) != 38 || s[0] != '{' || s[37] != '}' {
		return syscall.GUID{}, fmt.Errorf("invalid GUID format: %s", s)
	}
	d1, _ := strconv.ParseUint(s[1:9], 16, 32)
	d2, _ := strconv.ParseUint(s[10:14], 16, 16)
	d3, _ := strconv.ParseUint(s[15:19], 16, 16)
	var d4 [8]byte
	for i := 0; i < 8; i++ {
		start := 20 + i*2
		if i >= 2 {
			start = 21 + i*2
		}
		b, _ := strconv.ParseUint(s[start:start+2], 16, 8)
		d4[i] = byte(b)
	}
	return syscall.GUID{Data1: uint32(d1), Data2: uint16(d2), Data3: uint16(d3), Data4: d4}, nil
}

type iShellLinkWVtbl struct {
	QueryInterface, AddRef, Release               uintptr
	GetPath, GetIDList, SetIDList                  uintptr
	GetDescription, SetDescription                 uintptr
	GetWorkingDirectory, SetWorkingDirectory        uintptr
	GetArguments, SetArguments                      uintptr
	GetHotkey, SetHotkey                            uintptr
	GetShowCmd, SetShowCmd                          uintptr
	GetIconLocation, SetIconLocation                uintptr
	SetRelativePath, Resolve, SetPath               uintptr
}

type iShellLinkW struct{ lpVtbl *iShellLinkWVtbl }

type iPersistFileVtbl struct {
	QueryInterface, AddRef, Release uintptr
	GetClassID, IsDirty             uintptr
	Load, Save, SaveCompleted       uintptr
	GetCurFile                      uintptr
}

type iPersistFile struct{ lpVtbl *iPersistFileVtbl }

func createShortcut(shortcutPath, targetPath, arguments, workingDir string, windowStyle int) error {
	hr, _, _ := procCoInitializeEx.Call(0, 0)
	if hr != 0 {
		return fmt.Errorf("CoInitializeEx failed: %d", hr)
	}
	defer procCoUninitialize.Call()

	clsid, _ := parseGUID("{00021401-0000-0000-C000-000000000046}")
	iid, _ := parseGUID("{000214F9-0000-0000-C000-000000000046}")

	var shellLink *iShellLinkW
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, 1,
		uintptr(unsafe.Pointer(&iid)),
		uintptr(unsafe.Pointer(&shellLink)),
	)
	if hr != 0 {
		return fmt.Errorf("CoCreateInstance failed: %d", hr)
	}
	defer syscall.SyscallN(shellLink.lpVtbl.Release, uintptr(unsafe.Pointer(shellLink)))

	p, _ := syscall.UTF16PtrFromString(targetPath)
	syscall.SyscallN(shellLink.lpVtbl.SetPath, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(p)))

	p, _ = syscall.UTF16PtrFromString(arguments)
	syscall.SyscallN(shellLink.lpVtbl.SetArguments, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(p)))

	p, _ = syscall.UTF16PtrFromString(workingDir)
	syscall.SyscallN(shellLink.lpVtbl.SetWorkingDirectory, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(p)))

	syscall.SyscallN(shellLink.lpVtbl.SetShowCmd, uintptr(unsafe.Pointer(shellLink)), uintptr(windowStyle))

	persistIID, _ := parseGUID("{0000010b-0000-0000-C000-000000000046}")
	var persistFile *iPersistFile
	hr, _, _ = syscall.SyscallN(shellLink.lpVtbl.QueryInterface,
		uintptr(unsafe.Pointer(shellLink)),
		uintptr(unsafe.Pointer(&persistIID)),
		uintptr(unsafe.Pointer(&persistFile)),
	)
	if hr != 0 {
		return fmt.Errorf("QueryInterface IPersistFile failed: %d", hr)
	}
	defer syscall.SyscallN(persistFile.lpVtbl.Release, uintptr(unsafe.Pointer(persistFile)))

	p, _ = syscall.UTF16PtrFromString(shortcutPath)
	syscall.SyscallN(persistFile.lpVtbl.Save, uintptr(unsafe.Pointer(persistFile)), uintptr(unsafe.Pointer(p)), 1)

	return nil
}
