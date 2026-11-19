import (
	"vela/http"
	"strconv"
	"strings"
)

"get-random": {
	type: "source"
	annotations: {}
	labels: {}
	description: "Fetches a random integer in [min,max] from random.org, cached per (min,max) for storageTTL."
}
template: {
	// Output contract. value is the int form (e.g. for spec.replicas);
	// valueString is the raw string.
	schema: {
		value:       int
		valueString: string
	}

	// Cached per (min,max) for storageTTL. A random source is deliberately
	// not fetched every reconcile; the cache bounds calls to random.org.
	// Within a window every consumer sees the same number; it re-rolls on
	// the next miss. Force a re-roll with: vela config delete <key>.
	//
	// NOTE: an in-memory LRU (~30s, fixed) sits in front of this store, so
	// the effective floor on how often the value changes is roughly
	// storageTTL + 30s within a running controller. Setting storageTTL below
	// ~30s will not make it re-roll faster than the in-memory layer allows.
	storage: {
		storageTTL:     "10s"
		onStaleFailure: "use-stale"
	}

	parameter: {
		// +usage=Inclusive lower bound
		min: *1 | int
		// +usage=Inclusive upper bound
		max: *5 | int
	}

	// random.org returns a single integer as plain text (with a trailing
	// newline), not JSON -- so trim and parse rather than json.Unmarshal.
	_resp: http.#Get & {
		$params: url: "https://www.random.org/integers/?num=1&min=\(strconv.FormatInt(parameter.min, 10))&max=\(strconv.FormatInt(parameter.max, 10))&col=1&base=10&format=plain&rnd=new"
	}

	_trimmed: strings.TrimSpace(_resp.$returns.body)

	output: {
		valueString: _trimmed
		value:       strconv.Atoi(_trimmed)
	}
}
