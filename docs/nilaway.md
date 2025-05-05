~/go/bin/nilaway  -v -experimental-anonymous-function  -experimental-struct-init ./...
17:25:28.419375 load [./...]
17:25:29.860997 building graph of analysis passes
/home/lalbers/gitroot/irr/cmd/irr/override.go:753:78: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- irr/override.go:985:51: result 0 of `setupGeneratorConfig()` lacking guarding; passed as arg `config` to `createAndExecuteGenerator()` via the assignment(s):
		- `setupGeneratorConfig(...)` to `generatorConfig` at irr/override.go:963:2
	- irr/override.go:753:78: function parameter `config` accessed field `ChartPath`

/home/lalbers/gitroot/irr/pkg/chart/loader_test.go:86:28: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- loader/directory.go:119:10: uninitialized field `Metadata` escaped out of our analysis scope (presumed nilable)
	- chart/loader_test.go:86:28: field `Metadata` accessed field `Version`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:110:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:35:15: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:110:15: field `fs` called `Create()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:110:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:395:15: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:110:15: field `fs` called `Create()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:110:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:62:11: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:110:15: field `fs` called `Create()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:119:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` escaped out of our analysis scope (presumed nilable)
	- fileutil/utils_test.go:172:12: field `fs` field `fs` of method receiver `a`
	- fileutil/fs.go:119:9: field `fs` called `Mkdir()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:119:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:73:9: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:119:9: field `fs` called `Mkdir()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:119:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:93:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:119:9: field `fs` called `Mkdir()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:119:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` escaped out of our analysis scope (presumed nilable)
	- fileutil/utils_test.go:356:12: field `fs` field `fs` of method receiver `a`
	- fileutil/fs.go:119:9: field `fs` called `Mkdir()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:128:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:126:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:128:9: field `fs` called `MkdirAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:128:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` escaped out of our analysis scope (presumed nilable)
	- fileutil/utils_test.go:258:12: field `fs` field `fs` of method receiver `a`
	- fileutil/fs.go:128:9: field `fs` called `MkdirAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:128:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:132:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:128:9: field `fs` called `MkdirAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:128:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:117:9: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:128:9: field `fs` called `MkdirAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:137:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:409:14: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:137:15: field `fs` called `Open()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:137:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:161:11: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:137:15: field `fs` called `Open()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:137:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:147:15: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:137:15: field `fs` called `Open()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:146:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:170:15: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:146:15: field `fs` called `OpenFile()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:146:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:185:14: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:146:15: field `fs` called `OpenFile()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:146:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:198:11: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:146:15: field `fs` called `OpenFile()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:155:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:211:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:155:9: field `fs` called `Remove()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:155:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:220:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:155:9: field `fs` called `Remove()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:164:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:258:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:164:9: field `fs` called `RemoveAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:164:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:106:12: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:164:9: field `fs` called `RemoveAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:164:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:88:12: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:164:9: field `fs` called `RemoveAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:164:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:246:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:164:9: field `fs` called `RemoveAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:164:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:237:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:164:9: field `fs` called `RemoveAll()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:173:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:293:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:173:9: field `fs` called `Rename()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:173:9: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:275:8: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:173:9: field `fs` called `Rename()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:182:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:307:15: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:182:15: field `fs` called `Stat()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:182:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` escaped out of our analysis scope (presumed nilable)
	- fileutil/utils_test.go:130:15: field `fs` field `fs` of method receiver `a`
	- fileutil/fs.go:182:15: field `fs` called `Stat()`

/home/lalbers/gitroot/irr/pkg/fileutil/fs.go:182:15: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
	- fileutil/fs_test.go:21:19: literal `nil` passed as arg `fs` to `NewAferoFS()`
	- fileutil/fs.go:62:9: function parameter `fs` field `fs` returned by result 0 of `NewAferoFS()`
	- fileutil/fs_test.go:315:11: field `fs` of result 0 of `NewAferoFS()` field `fs` of method receiver `a`
	- fileutil/fs.go:182:15: field `fs` called `Stat()`

