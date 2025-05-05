[200~~/go/bin/nilaway -experimental-anonymous-function  -experimental-struct-init ./...
/home/lalbers/gitroot/irr/pkg/image/detector.go:549:16: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:333:53: result 0 of `genSplit()` returned from `SplitN()` in position 0
    - image/detector.go:549:16: result 0 of `SplitN()` sliced into

(Same nil source could also cause potential nil panic(s) at 1 other place(s): image/detector.go:564:16.)

/home/lalbers/gitroot/irr/pkg/strategy/path_strategy.go:81:80: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:333:53: result 0 of `genSplit()` returned from `SplitN()` in position 0
    - strategy/path_strategy.go:81:80: result 0 of `SplitN()` sliced into via the assignment(s):
        - `strings.SplitN(...)` to `parts` at strategy/path_strategy.go:80:5

(Same nil source could also cause potential nil panic(s) at 3 other place(s): strategy/path_strategy.go:81:98, strategy/path_strategy.go:86:13, and strategy/path_strategy.go:89:33.)

/home/lalbers/gitroot/irr/pkg/chart/generator.go:289:31: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:333:53: result 0 of `genSplit()` returned from `SplitN()` in position 0
    - chart/generator.go:289:31: result 0 of `SplitN()` sliced into via the assignment(s):
        - `strings.SplitN(...)` to `parts` at chart/generator.go:288:5

/home/lalbers/gitroot/irr/pkg/chart/generator.go:965:10: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:361:45: result 0 of `genSplit()` returned from `Split()` in position 0
    - chart/generator.go:965:10: result 0 of `Split()` sliced into via the assignment(s):
        - `strings.Split(...)` to `pathElems` at chart/generator.go:959:2

(Same nil source could also cause potential nil panic(s) at 1 other place(s): chart/generator.go:1044:14.)

/home/lalbers/gitroot/irr/pkg/chart/loader.go:122:71: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - loader/directory.go:119:10: uninitialized field `Metadata` escaped out of our analysis scope (presumed nilable)
    - chart/loader.go:122:71: field `Metadata` accessed field `Version`

/home/lalbers/gitroot/irr/pkg/analyzer/analyzer.go:207:20: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:333:53: result 0 of `genSplit()` returned from `SplitN()` in position 0
    - analyzer/analyzer.go:207:20: result 0 of `SplitN()` sliced into via the assignment(s):
        - `strings.SplitN(...)` to `repoParts` at analyzer/analyzer.go:206:7

(Same nil source could also cause potential nil panic(s) at 1 other place(s): analyzer/analyzer.go:223:19.)

/home/lalbers/gitroot/irr/cmd/irr/override.go:708:16: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - irr/override.go:743:96: result 0 of `getValuesOptionsFromFlags()` lacking guarding; passed as arg `valueOpts` to `performContextAwareAnalysis()` via the assignment(s):
        - `getValuesOptionsFromFlags(cmd)` to `valueOpts` at irr/override.go:736:2
    - irr/override.go:708:16: function parameter `valueOpts` dereferenced

/home/lalbers/gitroot/irr/cmd/irr/override.go:788:16: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - irr/override.go:961:51: result 0 of `setupGeneratorConfig()` lacking guarding; passed as arg `config` to `createAndExecuteGenerator()` via the assignment(s):
        - `setupGeneratorConfig(...)` to `generatorConfig` at irr/override.go:939:2
    - irr/override.go:788:16: function parameter `config` accessed field `ChartPath`

/home/lalbers/gitroot/irr/pkg/version/version.go:28:11: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:361:45: result 0 of `genSplit()` returned from `Split()` in position 0
    - version/version.go:28:11: result 0 of `Split()` sliced into

/home/lalbers/gitroot/irr/test/integration/harness.go:609:11: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - strings/strings.go:293:10: literal `nil` returned from `genSplit()` in position 0
    - strings/strings.go:333:53: result 0 of `genSplit()` returned from `SplitN()` in position 0
    - integration/harness.go:609:11: result 0 of `SplitN()` sliced into via the assignment(s):
        - `strings.SplitN(...)` to `parts` at integration/harness.go:605:3

/home/lalbers/gitroot/irr/pkg/chart/generator_test.go:733:17: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - chart/generator_test.go:733:17: unassigned variable `procErr` accessed field `Errors`

(Same nil source could also cause potential nil panic(s) at 1 other place(s): chart/generator_test.go:734:21.)

/home/lalbers/gitroot/irr/pkg/chart/loader.go:122:71: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - loader/directory.go:119:10: uninitialized field `Metadata` escaped out of our analysis scope (presumed nilable)
    - chart/loader.go:122:71: field `Metadata` accessed field `Version`

(Same nil source could also cause potential nil panic(s) at 1 other place(s): chart/loader_test.go:86:28.)

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

/home/lalbers/gitroot/irr/pkg/helm/client_test.go:96:32: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - loader/directory.go:119:10: uninitialized field `Metadata` escaped out of our analysis scope (presumed nilable)
    - helm/client_test.go:96:32: field `Metadata` accessed field `Name`

(Same nil source could also cause potential nil panic(s) at 2 other place(s): helm/client_test.go:97:27, and helm/client_test.go:217:32.)

/home/lalbers/gitroot/irr/pkg/helm/sdk_test.go:34:13: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - mock/mock.go:114:9: read directly from variadic parameter `returnArguments` field `ReturnArguments` escaped out of our analysis scope (presumed nilable) via the assignment(s):
        - `returnArguments` to `c.ReturnArguments` at mock/mock.go:112:2
    - mock/mock.go:577:9: field `ReturnArguments` returned from `MethodCalled()` in position 0 via the assignment(s):
        - `call.ReturnArguments` to `returnArgs` at mock/mock.go:574:2
    - mock/mock.go:481:9: result 0 of `MethodCalled()` returned from `Called()` in position 0
    - helm/sdk_test.go:34:13: result 0 of `Called()` called `Get()` via the assignment(s):
        - `m.Called()` to `args` at helm/sdk_test.go:33:2

(Same nil source could also cause potential nil panic(s) at 8 other place(s): helm/sdk_test.go:47:5, helm/sdk_test.go:48:44, helm/sdk_test.go:50:13, helm/sdk_test.go:54:43, helm/sdk_test.go:59:5, helm/sdk_test.go:60:44, helm/sdk_test.go:62:13, and helm/sdk_test.go:66:43.)

/home/lalbers/gitroot/irr/pkg/helm/sdk_test.go:413:46: error: Potential nil panic detected. Observed nil flow from source to dereference point: 
    - repo/repo.go:55:9: uninitialized field `Repositories` escaped out of our analysis scope (presumed nilable)
    - helm/sdk_test.go:413:46: field `Repositories` sliced into

(Same nil source could also cause potential nil panic(s) at 2 other place(s): helm/sdk_test.go:428:46, and helm/sdk_test.go:446:54.)

