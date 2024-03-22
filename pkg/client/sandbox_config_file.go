package client

import (
	"os"
	"path"
)

func EnsureKsctlConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dirPath := path.Join(home, ".kube")
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.Mkdir(dirPath, os.ModePerm); err != nil {
			return "", err
		}
	}
	filePath := path.Join(dirPath, "ksctl-config")
	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		emptyFile, err := os.Create(filePath)
		if err != nil {
			return "", err
		}
		if err := emptyFile.Close(); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return filePath, nil
}
