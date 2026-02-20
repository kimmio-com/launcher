cd ../

$(go env GOPATH)/bin/reflex \
  -r '\.(go|html|css|js)$' \
  -s \
  -- sh -c 'go build -o kimmo ./cmd/luncher && ./kimmo'
