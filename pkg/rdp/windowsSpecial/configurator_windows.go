//go:build windows
// +build windows

package windowsspecial

import (
	"fmt"
	"log"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"github.com/zieckey/goini"
)

type WindowsConfigurator struct{}

func (configurator *WindowsConfigurator) GetConfig() (*rdp.StreamConfig, error) {
	// TODO OS Selection
	ini := goini.New()
	err := ini.ParseFile("C:\\ProgramData\\e8rd\\config.ini")
	if err != nil {
		log.Printf("Config Parse Error")
		return &rdp.StreamConfig{}, err
	}
	// todo error checking
	config := &rdp.StreamConfig{}
	ok := false
	//config.os, ok = ini.SectionGet("OS", "os")
	//if !ok {
	//log.Printf("Invalid OS Parameter")
	//return nil, fmt.Errorf("could not find os in config")
	//}
	config.Codec, ok = ini.SectionGet("Encoding", "codec")
	if !ok {
		log.Printf("Invalid Codec Parameter")
		return nil, fmt.Errorf("could not find codec in config")
	}
	config.Encoder, ok = ini.SectionGet("Encoding", "encoder")
	if !ok {
		log.Printf("Invalid Encoder Parameter")
		return nil, fmt.Errorf("could not find encoder in config")
	}
	config.Bitrate, ok = ini.SectionGetInt("Encoding", "bitrate")
	if !ok {
		log.Printf("Invalid Bitrate Parameter")
		return nil, fmt.Errorf("could not find bitrate in config")
	}
	config.Gopsize, ok = ini.SectionGetInt("Encoding", "gopsize")
	if !ok {
		log.Printf("Invalid GOP Size Parameter")
		return nil, fmt.Errorf("could not find gopsize in config")
	}
	config.Screen, ok = ini.SectionGetInt("Capture", "screen")
	if !ok {
		log.Printf("Invalid Screen Index Parameter")
		return nil, fmt.Errorf("could not find monitor index in config")
	}
	config.Framerate, ok = ini.SectionGetInt("Capture", "framerate")
	if !ok {
		log.Printf("Invalid Framerate Parameter")
		return nil, fmt.Errorf("could not find fps in config")
	}
	config.MTU, ok = ini.SectionGetInt("Stream", "mtu")
	if !ok {
		log.Printf("Invalid MTU Parameter")
		return nil, fmt.Errorf("could not find mtu in config")
	}
	config.ApiURL, ok = ini.SectionGet("Server", "url")
	if !ok {
		log.Printf("Invalid API URL Parameter")
		return nil, fmt.Errorf("could not find api url in config")
	}
	config.Hostname, ok = ini.SectionGet("Server", "name")
	if !ok {
		log.Printf("Invalid API URL Parameter")
		return nil, fmt.Errorf("could not find hostname in config")
	}
	config.Token, ok = ini.SectionGet("Server", "token")
	if !ok {
		log.Printf("Invalid API token Parameter")
		return nil, fmt.Errorf("could not find hostname in config")
	}

	return config, nil
}
