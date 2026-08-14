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
