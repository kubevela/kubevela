# config-store

Installs a shared ConfigMap and registers a `config-store` component type.

Second fixture for the addon-as-component e2e suite. Deliberately cheap to
install — no images to pull — so the multi-addon bundle specs are not slowed
down by a second workload.
