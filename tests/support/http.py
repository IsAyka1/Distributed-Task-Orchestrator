from dataclasses import dataclass
from http.client import HTTPConnection
import json
import selectors


@dataclass(frozen=True)
class HTTPResponse:
    status: int
    body: dict
    content_type: str


@dataclass(frozen=True)
class HTTPClient:
    address: str

    @staticmethod
    def read_signal(process):
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            assert selector.select(timeout=5), "HTTP probe did not announce the expected signal"
            return process.stdout.readline().strip()

    @classmethod
    def from_process(cls, process):
        line = cls.read_signal(process)
        assert line.startswith("HTTP listening on 127.0.0.1:"), "HTTP server startup failed"
        return cls(line.removeprefix("HTTP listening on "))

    def request(self, path, document=None, *, raw=None, media="application/json", chunked=False, method="POST"):
        body = raw if raw is not None else json.dumps(document).encode()
        conn = HTTPConnection(self.address, timeout=5)
        try:
            headers = {} if media is None else {"Content-Type": media}
            if chunked:
                body = [body[i:i+4096] for i in range(0, len(body), 4096)]
            conn.request(method, path, body, headers, encode_chunked=chunked)
            response = conn.getresponse()
            return HTTPResponse(response.status, json.loads(response.read()), response.getheader("Content-Type"))
        finally:
            conn.close()

    def created(self, path, document):
        response = self.request(path, document)
        assert response.status == 201, response
        assert response.content_type == "application/json"
        return response.body
