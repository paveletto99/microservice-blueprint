# ---- Certificates ----

HTTP/3 always operates using TLS, meaning that running a WebTransport over
HTTP/3 server requires a valid TLS certificate. The easiest way to do this
is to get a certificate from a real publicly trusted CA like
<https://letsencrypt.org/>.

## 1. Generate a certificate and a private key

```shell
openssl req -newkey rsa:2048 -nodes -keyout certificate.key \
 -x509 -out certificate.pem -subj '/CN=Test Certificate' \
 -addext "subjectAltName = DNS:localhost"
```

## 2. Compute the fingerprint of the certificate

````shell
openssl x509 -pubkey -noout -in certificate.pem |
  openssl rsa -pubin -outform der |
  openssl dgst -sha256 -binary | base64
```

# The result should be a base64-encoded blob that looks like this:

# "3kdq3tazd08vjt1C50GE2sq9WLfw2W8KKX0I6YVsagM="
````
