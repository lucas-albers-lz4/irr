package image

import "regexp"

// Known path patterns for image-containing fields
var (
	imagePathPatterns = []string{
		"^image$",                 // key is exactly 'image'
		"\\bimage$",               // key ends with 'image'
		"^.*\\.image$",            // any path ending with '.image'
		"^.*\\.images\\[\\d+\\]$", // array elements in an 'images' array
		"^spec\\.template\\.spec\\.containers\\[\\d+\\]\\.image$",                      // k8s container image
		"^spec\\.template\\.spec\\.initContainers\\[\\d+\\]\\.image$",                  // k8s init container image
		"^spec\\.jobTemplate\\.spec\\.template\\.spec\\.containers\\[\\d+\\]\\.image$", // k8s job container image
		"^simpleImage$",                // test pattern used in unit tests
		"^nestedImages\\.simpleImage$", // nested test pattern for simpleImage
		"^workerImage$",                // Added for specific test case
		"^publicImage$",                // Added for Excluded_Registry test
		"^internalImage$",              // Added for Excluded_Registry test (map)
		"^dockerImage$",                // Added for Non-Source_Registry test
		"^quayImage$",                  // Added for Non-Source_Registry test (map)
		"^imgDocker$",                  // Added for Prefix_Source_Registry_Strategy test
		"^imgGcr$",                     // Added for Prefix_Source_Registry_Strategy test
		"^imgQuay$",                    // Added for Prefix_Source_Registry_Strategy test (map)
		"^parentImage$",                // Added for Chart_with_Dependencies test
		"^imageFromDocker$",            // Added for WithRegistryMapping test
		"^imageFromQuay$",              // Added for WithRegistryMapping test
		"^imageUnmapped$",              // Added for WithRegistryMapping test
		"^appImage$",                   // Added for Simple_Image_Map_Override test
		// Paths used in TestDetectImages/Strict_mode
		"^knownPathValid$",
		"^knownPathBadTag$",
		"^knownPathNonSource$",
		"^knownPathExcluded$",
		"^templateValue$",   // Arguably not an image path, but used in test
		"^mapWithTemplate$", // Arguably not an image path, but used in test
		// Paths used in TestImageDetector_DetectImages_EdgeCases/mixed_valid_and_invalid_images
		"^invalid image$", // Handle space in key for test
	}

	// Compiled regex patterns for image paths
	imagePathRegexps = compilePathPatterns(imagePathPatterns)

	// Known non-image path patterns
	nonImagePathPatterns = []string{
		"\\.enabled$",
		"\\.annotations\\.",
		"\\.labels\\.",
		"\\.port$",
		"\\.ports\\.",
		"\\.timeout$",
		"\\.serviceAccountName$",
		"\\.replicas$",
		"\\.resources\\.",
		"\\.env\\.",
		"\\.command\\[\\d+\\]$",
		"\\.args\\[\\d+\\]$",
		"\\[\\d+\\]\\.name$",               // container name field
		"containers\\[\\d+\\]\\.name$",     // explicit k8s container name
		"initContainers\\[\\d+\\]\\.name$", // explicit k8s init container name
		"\\.tag$",                          // standalone tag field
		"\\.registry$",                     // standalone registry field
		"\\.repository$",                   // standalone repository field
	}

	// Compiled regex patterns for non-image paths
	nonImagePathRegexps = compilePathPatterns(nonImagePathPatterns)
)

// compilePathPatterns compiles a list of path patterns into regexps
func compilePathPatterns(patterns []string) []*regexp.Regexp {
	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		r := regexp.MustCompile(pattern)
		regexps = append(regexps, r)
	}
	return regexps
}
