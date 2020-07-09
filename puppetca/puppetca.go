package puppetca

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// Client is a Puppet CA client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func isFile(str string) bool {
	if strings.Contains(str, "-BEGIN CERTIFICATE-") || strings.Contains(str, "-BEGIN RSA PRIVATE KEY-") {
		return false
	}
	return strings.HasSuffix(str, ".pem") || strings.HasSuffix(str, ".cer") || strings.HasSuffix(str, ".key") || strings.HasPrefix(str, "/") || strings.HasPrefix(str, "./") || strings.HasPrefix(str, "../")
}

// NewClient returns a new Client
func NewClient(baseURL, keyStr, certStr, caStr string, ignoreSsl bool) (c Client, err error) {
	// Load client cert
	var cert tls.Certificate
	if isFile(certStr) {
		if !isFile(keyStr) {
			err = fmt.Errorf("cert points to a file but key is a string")
			return
		}

		cert, err = tls.LoadX509KeyPair(certStr, keyStr)
		if err != nil {
			err = errors.Wrapf(err, "failed to load client cert from file %s", certStr)
			return c, err
		}
	} else {
		if isFile(keyStr) {
			err = fmt.Errorf("cert is a string but key points to a file")
			return c, err
		}

		cert, err = tls.X509KeyPair([]byte(certStr), []byte(keyStr))
		if err != nil {
			err = errors.Wrapf(err, "failed to load client cert from string")
			return c, err
		}
	}

	// Load CA cert
	var caCert []byte
	if isFile(caStr) {
		caCert, err = ioutil.ReadFile(caStr)
		if err != nil {
			err = errors.Wrapf(err, "failed to load CA cert at %s", caStr)
			return
		}
	} else {
		caCert = []byte(caStr)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: ignoreSsl,
	}
	tr := &http.Transport{TLSClientConfig: tlsConfig}
	httpClient := &http.Client{Transport: tr}
	c = Client{baseURL, httpClient}

	return
}

// GetCertByName returns the certificate of a node by its name
func (c *Client) GetCertByName(nodename string) (string, error) {
	certInfo, err := c.Get(fmt.Sprintf("certificate_status/%s", nodename))
	if err != nil {
		return "", errors.Wrapf(err, "failed to retrieve certificate %s", nodename)
	}
	return certInfo, nil
}

// DeleteCertByName deletes the certificate of a given node
func (c *Client) DeleteCertByName(nodename string) error {
	_, err := c.Delete(fmt.Sprintf("certificate_status/%s", nodename))
	if err != nil {
		return errors.Wrapf(err, "failed to delete certificate %s", nodename)
	}
	return nil
}

// SignCertByName signs the certificate of a given node
func (c *Client) SignCertByName(nodename string) error {
	payload := "{\"desired_state\":\"signed\"}"
	_, err := c.Put(fmt.Sprintf("certificate_status/%s", nodename), payload)
	if err != nil {
		return errors.Wrapf(err, "failed to sign certificate %s", nodename)
	}
	return nil
}

// Get performs a GET request
func (c *Client) Get(path string) (string, error) {
	return c.Do("GET", path, "")
}

// Delete performs a DELETE request
func (c *Client) Delete(path string) (string, error) {
	return c.Do("DELETE", path, "")
}

// Put performs a PUT request
func (c *Client) Put(path string, payload string) (string, error) {
	return c.Do("PUT", path, payload)
}

// Do performs an HTTP request
func (c *Client) Do(method, path string, payload string) (string, error) {
	fullPath := fmt.Sprintf("%s/puppet-ca/v1/%s", c.baseURL, path)
	uri, err := url.Parse(fullPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse URL %s", fullPath)
	}
	req := http.Request{
		Method: method,
		URL:    uri,
	}
	if payload != "" {
		req.Header = make(http.Header)
		req.Header.Add("Content-Type", "text/pson")
		req.Body = ioutil.NopCloser(strings.NewReader(payload))
	}
	resp, err := c.httpClient.Do(&req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to %s URL %s", method, fullPath)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("failed to %s URL %s, got: %s", method, fullPath, resp.Status)
	}
	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read body response from %s")
	}

	return string(content), nil
}
