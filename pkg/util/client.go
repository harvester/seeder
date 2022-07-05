package util

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	certutil "github.com/rancher/dynamiclistener/cert"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// FetchKubeConfig is a helper method to fetch remote harvester clusters kubeconfig file
func FetchKubeConfig(serverURL, prefix, token string) ([]byte, error) {
	kubeletURL := fmt.Sprintf("%s/v1-%s/client-kubelet.crt", serverURL, prefix)
	resp, err := fetchCerts(kubeletURL, prefix, token)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	certByte, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	certs, err := certutil.ParseCertsPEM(certByte)
	if err != nil {
		return nil, err
	}

	if len(certs) != 2 {
		return nil, fmt.Errorf("expected to find two certs, but found %d", len(certs))
	}

	var kubeletcert *x509.Certificate
	for _, c := range certs {
		if !c.IsCA {
			kubeletcert = c
		}
	}

	kubeletkey, err := certutil.ParsePrivateKeyPEM(certByte)
	if err != nil {
		return nil, err
	}

	ecdsaPrivKey, err := x509.MarshalECPrivateKey(kubeletkey.(*ecdsa.PrivateKey))
	if err != nil {
		return nil, err
	}
	kubeletKeyBlock := &pem.Block{
		Bytes: ecdsaPrivKey,
		Type:  certutil.ECPrivateKeyBlockType,
	}

	keyBytes := pem.EncodeToMemory(kubeletKeyBlock)
	certBytes := certutil.EncodeCertPEM(kubeletcert)

	// fetch ca cert
	caURL := fmt.Sprintf("%s/v1-%s/server-ca.crt", serverURL, prefix)
	resp, err = fetchCerts(caURL, prefix, token)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	caCertByte, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	config := clientcmdapi.NewConfig()

	cluster := clientcmdapi.NewCluster()
	cluster.CertificateAuthorityData = caCertByte
	cluster.Server = serverURL
	//cluster.InsecureSkipTLSVerify = true

	authInfo := clientcmdapi.NewAuthInfo()
	authInfo.ClientCertificateData = certBytes
	authInfo.ClientKeyData = keyBytes

	context := clientcmdapi.NewContext()
	context.AuthInfo = "default"
	context.Cluster = "default"

	config.Clusters["default"] = cluster
	config.AuthInfos["default"] = authInfo
	config.Contexts["default"] = context
	config.CurrentContext = "default"
	return clientcmd.Write(*config)
}

func fetchCerts(url, prefix, token string) (*http.Response, error) {
	c := &http.Client{Transport: &http.Transport{
		IdleConnTimeout: 30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
		Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add(fmt.Sprintf("%s-Node-Name", prefix), "tmp")
	req.Header.Add(fmt.Sprintf("%s-Node-Password", prefix), token)
	req.SetBasicAuth("node", token)
	resp, err := c.Do(req)
	return resp, err
}
