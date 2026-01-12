.PHONY: build linux clean install uninstall

build:
	go build -o messageboy .

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o messageboy-linux .

clean:
	rm -f messageboy messageboy-linux

install:
	mkdir -p /opt/messageboy
	cp messageboy-linux /opt/messageboy/messageboy
	test -f /opt/messageboy/config.json || cp config.json /opt/messageboy/
	cp messageboy.service /etc/systemd/system/
	systemctl daemon-reload
	systemctl enable messageboy

uninstall:
	systemctl stop messageboy || true
	systemctl disable messageboy || true
	rm -f /etc/systemd/system/messageboy.service
	rm -rf /opt/messageboy
	systemctl daemon-reload
