import "vela/http"

req: http.#Get & {
  $params: {
    url: "https://cuelang.org"
  }
}

body: req.$returns.body
statusCode: req.$returns.statusCode
