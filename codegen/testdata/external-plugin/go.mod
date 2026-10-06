module example.com/independent-backend

go 1.24

require github.com/relux-works/javacard-rpc/pluginapi v0.1.0

// Unpublished candidate only; remove after pluginapi/v0.1.0 publication.
replace github.com/relux-works/javacard-rpc/pluginapi => ../../../pluginapi
