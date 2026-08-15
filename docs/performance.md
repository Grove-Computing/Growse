# Performance Baseline

v0.6.0では48 card・6 columnの固定Dashboard fixtureを代表負荷とし、Layout passとrenderer-independent Paint passを分離して計測する。

## 実行方法

```sh
go test ./internal/layout ./internal/paint -run '^$' \
  -bench 'Benchmark(GridDashboardLayout|DashboardPaint)$' \
  -benchmem -count=5
```

比較時は同じmachine、Go version、power profileを使用し、単発値ではなく5回の中央値を見る。LayoutにはStyle計算を含めず、PaintにはLayout計算を含めない。

## v0.6.0 baseline

- 計測日: 2026-08-14
- OS: Linux 7.0.0-28-generic / amd64
- CPU: AMD Ryzen 7 8745HS with Radeon 780M Graphics
- Go: 1.26.6

| Benchmark | 中央値 | Memory | Allocations |
|---|---:|---:|---:|
| `BenchmarkGridDashboardLayout-16` | 1,112,829 ns/op | 3,214,364 B/op | 4,600 allocs/op |
| `BenchmarkDashboardPaint-16` | 19,432 ns/op | 43,704 B/op | 201 allocs/op |

baselineはrelease間の傾向把握用であり、異なるhost間の合否判定には使用しない。意図しない退行を調査するときは`benchstat`で複数回の結果を比較する。

## v0.8.0 Form、Cookie、Fetch workload

2026-08-14にLinux amd64、AMD Ryzen 7 8745HS、Go 1.26.6で次を実行した。

```sh
go test ./internal/forms ./internal/network ./internal/webapi/fetch \
  -run '^$' \
  -bench 'Benchmark(Serialize100FormControls|Match1000Cookies|Complete16ConcurrentFetches)$' \
  -benchmem -count=1
```

| Benchmark | Workload | Baseline | Memory | Allocations |
|---|---:|---:|---:|---:|
| `BenchmarkSerialize100FormControls-16` | 100 controls | 22,663 ns/op | 25,872 B/op | 617 allocs/op |
| `BenchmarkMatch1000Cookies-16` | 1,000 cookies | 396,559 ns/op | 815,688 B/op | 1,027 allocs/op |
| `BenchmarkComplete16ConcurrentFetches-16` | 16 concurrent completions | 14,205 ns/op | 14,950 B/op | 225 allocs/op |

Cookie benchmarkは安全上限をtest用に1,200へ設定し、同一Domain/Pathの1,000 Cookie matchingを測定する。Fetch benchmarkはHTTP transportを注入fakeへ置換し、goroutine開始からPage callback queueでの完了までを測定する。

## v0.7.0 Animation workload

100要素がそれぞれ1つのinfinite Animationを持つ状態で、同一timestampの1 Frameをsampleする。DOM走査、Style再計算、Layout、Paintは含めず、Animation RegistryとTiming計算の費用を測定する。

```sh
go test ./internal/style -run '^$' \
  -bench '^BenchmarkSample100ElementAnimations$' \
  -benchmem -count=5
```

基準値は上記v0.6.0 baselineと同じ環境で計測し、5回の中央値を記録する。

| Benchmark | 同時Animation数 | 中央値 | Memory | Allocations |
|---|---:|---:|---:|---:|
| `BenchmarkSample100ElementAnimations-16` | 100 | 5,908 ns/op | 4,800 B/op | 100 allocs/op |

## v0.9.0 Scheduler、History、Storage、HTTP Cache workload

2026-08-15にLinux amd64、AMD Ryzen 7 8745HS、Go 1.26.6で5回測定し、中央値を記録した。

```sh
go test ./internal/webapi/scheduler ./internal/browser ./internal/storage ./internal/network \
  -run '^$' \
  -bench 'Benchmark(RegisterAndClear10000Timers|Traverse1000HistoryEntries|LookupAndUpdate10000StorageEntries|HitAndRevalidate1000HTTPCacheEntries)$' \
  -benchmem -count=5
```

| Benchmark | Workload | 中央値 | Memory | Allocations |
|---|---:|---:|---:|---:|
| `BenchmarkRegisterAndClear10000Timers-16` | 10,000 Timer登録・解除 | 3,139,391 ns/op | 1,944,645 B/op | 10,107 allocs/op |
| `BenchmarkTraverse1000HistoryEntries-16` | 1,000 entryのBack / Forward往復 | 121,285 ns/op | 415,586 B/op | 3,996 allocs/op |
| `BenchmarkLookupAndUpdate10000StorageEntries-16` | 10,000 lookup・1 update | 414,763 ns/op | 327,680 B/op | 1 alloc/op |
| `BenchmarkHitAndRevalidate1000HTTPCacheEntries-16` | 500 fresh hit・500 validator生成 | 641,991 ns/op | 676,012 B/op | 7,500 allocs/op |

Timerは手動Clock、Historyはmemory entry、Storageはmemory Area、HTTP Cacheはmemory frontを使用し、Disk I/O・Network・実時間待機を含めない。これらの値は同じmachine上でrelease間の傾向を比較するbaselineであり、異なるhostの合否判定には使用しない。

## v0.10.0 Multi-Tab workload

2026-08-15にLinux amd64、AMD Ryzen 7 8745HS、Go 1.26.6で5回測定し、中央値を記録した。

```sh
go test ./internal/browser ./internal/storage ./internal/network \
  -run '^$' \
  -bench 'Benchmark(CreateSwitchAndClose64Tabs|DispatchLocalStorageUpdatesAcross16Tabs|SharedHTTPCacheHitAcross64Tabs)$' \
  -benchmem -count=5
```

| Benchmark | Workload | 中央値 | Memory | Allocations |
|---|---:|---:|---:|---:|
| `BenchmarkCreateSwitchAndClose64Tabs-16` | 64 Tab作成・全Tab切替・終了 | 27,465 ns/op | 33,896 B/op | 520 allocs/op |
| `BenchmarkDispatchLocalStorageUpdatesAcross16Tabs-16` | 16 TabからのLocal Storage更新とpeer配送 | 6,749 ns/op | 4,375 B/op | 47 allocs/op |
| `BenchmarkSharedHTTPCacheHitAcross64Tabs-16` | 64 Tab相当のfresh Cache hit | 35,966 ns/op | 51,200 B/op | 576 allocs/op |

Tab lifecycleはNetworkやPage parseを含まないSession model、Storage Eventは16 subscriberを持つmemory Area、HTTP Cacheは共有memory frontを使用する。baselineは同一machine上のrelease間比較に使い、異なるhostの合否判定には使用しない。
