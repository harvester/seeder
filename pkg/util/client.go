package util

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	certutil "github.com/rancher/dynamiclistener/cert"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// Config is an internal place holder to hold bootstrap config from server
// and is used to sign an admin certificate

type Config struct {
	ServerCA      []byte
	InternalCA    []byte
	InternalCAKey []byte
}

// Generate kubeconfig impersontates as a server and renders an admin kubeconfig which can be used to monitor and patch clusters
func GenerateKubeConfig(serverURL, port, prefix, token string) ([]byte, error) {

	c := &http.Client{Transport: &http.Transport{
		IdleConnTimeout: 30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
		Timeout: 5 * time.Second}

	configURL := fmt.Sprintf("https://%s:%s/v1-%s/server-bootstrap", serverURL, port, prefix)
	req, err := http.NewRequest("GET", configURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("server", token)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.Status != "200 OK" {
		return nil, fmt.Errorf("expected status code 200, got %s", resp.Status)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	type respStruct struct {
		TimeStamp string
		Content   string
	}

	respMap := make(map[string]respStruct)
	err = json.Unmarshal(content, &respMap)
	if err != nil {
		return nil, err
	}

	serverCAByte, err := base64.StdEncoding.DecodeString(respMap["ServerCA"].Content)
	if err != nil {
		return nil, err
	}

	internalCAByte, err := base64.StdEncoding.DecodeString(respMap["ClientCA"].Content)
	if err != nil {
		return nil, err
	}

	internalCAKeyByte, err := base64.StdEncoding.DecodeString(respMap["ClientCAKey"].Content)
	if err != nil {
		return nil, err
	}
	serverConfig := &Config{
		ServerCA:      serverCAByte,
		InternalCA:    internalCAByte,
		InternalCAKey: internalCAKeyByte,
	}

	// override to assist with unit tests
	apiPort := "6443"
	if port != seederv1alpha1.DefaultAPIPort {
		apiPort = port
	}
	return renderKubeConfig(serverConfig, serverURL, apiPort)
}

// GenerateKubeConfig will generate an admin kubeconfig using the serverconfig generated
func renderKubeConfig(c *Config, serverURL, port string) ([]byte, error) {
	// rke2 k8s api and registration ports are different
	// 9345 for registration
	// 6443 for apiserver
	// need to change the port in serverURL

	adminTemplateKey, err := certutil.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	certConfig := certutil.Config{
		CommonName:   "admin",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		ExpiresAt:    12 * time.Hour,
	}

	internalCAs, err := certutil.ParseCertsPEM(c.InternalCA)
	if err != nil {
		return nil, fmt.Errorf("error parsing internalCA: %v", err)
	}

	internalCAKey, err := certutil.ParsePrivateKeyPEM(c.InternalCAKey)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key for InternalCA: %v", err)
	}
	adminCert, err := certutil.NewSignedCert(certConfig, adminTemplateKey, internalCAs[0], internalCAKey.(crypto.Signer))
	if err != nil {
		return nil, err
	}

	adminCertBytes := certutil.EncodeCertPEM(adminCert)
	adminKeyBytes := certutil.EncodePrivateKeyPEM(adminTemplateKey)
	// package kubeconfig
	config := clientcmdapi.NewConfig()

	cluster := clientcmdapi.NewCluster()
	cluster.CertificateAuthorityData = c.ServerCA
	cluster.Server = fmt.Sprintf("https://%s:%s", serverURL, port)
	//cluster.InsecureSkipTLSVerify = true

	authInfo := clientcmdapi.NewAuthInfo()
	authInfo.ClientCertificateData = adminCertBytes
	authInfo.ClientKeyData = adminKeyBytes

	context := clientcmdapi.NewContext()
	context.AuthInfo = "default"
	context.Cluster = "default"

	config.Clusters["default"] = cluster
	config.AuthInfos["default"] = authInfo
	config.Contexts["default"] = context
	config.CurrentContext = "default"
	return clientcmd.Write(*config)
}
