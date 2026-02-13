package relay

import (
	"fmt"
	"sync"
	"testing"
)

// BenchmarkRouteLookup tests sync.Map route lookup with 10K routes
func BenchmarkRouteLookup(b *testing.B) {
	s := NewWSServer()

	// Populate 10K routes
	for i := uint32(0); i < 10000; i++ {
		src := fmt.Sprintf("client-%d", i%100)
		tgt := fmt.Sprintf("client-%d", (i+1)%100)
		route := &RouteInfo{
			SourceClientID: src,
			TargetClientID: tgt,
			StreamID:       i,
			ExitAddr:       fmt.Sprintf("192.168.1.%d:%d", i%256, 8000+i%1000),
		}
		s.routes.Store(routeKey(src, i), route)
		s.routes.Store(routeKey(tgt, i), route)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := uint32(0)
		for pb.Next() {
			sid := i % 10000
			s.routes.Load(routeKey(fmt.Sprintf("client-%d", sid%100), sid))
			i++
		}
	})
}

// BenchmarkRouteLookup_MixedReadWrite tests route lookup with concurrent writes
func BenchmarkRouteLookup_MixedReadWrite(b *testing.B) {
	s := NewWSServer()

	// Pre-populate
	for i := uint32(0); i < 1000; i++ {
		route := &RouteInfo{
			SourceClientID: "src",
			TargetClientID: "tgt",
			StreamID:       i,
		}
		s.routes.Store(routeKey("src", i), route)
		s.routes.Store(routeKey("tgt", i), route)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := uint32(0)
		for pb.Next() {
			sid := i % 1000
			if i%10 == 0 {
				// 10% writes
				route := &RouteInfo{
					SourceClientID: "src",
					TargetClientID: "tgt",
					StreamID:       sid,
				}
				s.routes.Store(routeKey("src", sid), route)
				s.routes.Store(routeKey("tgt", sid), route)
			} else {
				// 90% reads
				s.routes.Load(routeKey("src", sid))
			}
			i++
		}
	})
}

// BenchmarkRouteCleanup tests cleanup performance
func BenchmarkRouteCleanup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		s := NewWSServer()
		// Add 100 routes for "target-client"
		for j := uint32(0); j < 100; j++ {
			route := &RouteInfo{
				SourceClientID: "source-client",
				TargetClientID: "target-client",
				StreamID:       j,
			}
			s.routes.Store(routeKey("source-client", j), route)
			s.routes.Store(routeKey("target-client", j), route)
		}
		// Add 100 routes for other clients
		for j := uint32(100); j < 200; j++ {
			route := &RouteInfo{
				SourceClientID: "other-client",
				TargetClientID: "another-client",
				StreamID:       j,
			}
			s.routes.Store(routeKey("other-client", j), route)
			s.routes.Store(routeKey("another-client", j), route)
		}
		b.StartTimer()

		s.cleanupRoutesForClient("target-client")
	}
}

// BenchmarkRouteStoreAndDelete tests store/delete cycle
func BenchmarkRouteStoreAndDelete(b *testing.B) {
	s := NewWSServer()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := uint32(0)
		for pb.Next() {
			streamID := i % 10000
			route := &RouteInfo{
				SourceClientID: "src",
				TargetClientID: "tgt",
				StreamID:       streamID,
			}
			s.routes.Store(routeKey("src", streamID), route)
			s.routes.Store(routeKey("tgt", streamID), route)
			s.routes.Delete(routeKey("src", streamID))
			s.routes.Delete(routeKey("tgt", streamID))
			i++
		}
	})
}

// BenchmarkCleanupRoute tests cleanupRoute with load balancer/traffic counter
func BenchmarkCleanupRoute(b *testing.B) {
	s := NewWSServer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		streamID := uint32(i)
		route := &RouteInfo{
			SourceClientID: "src",
			TargetClientID: "tgt",
			StreamID:       streamID,
		}
		s.routes.Store(routeKey("src", streamID), route)
		s.routes.Store(routeKey("tgt", streamID), route)
		s.cleanupRoute(route)
	}
}

// BenchmarkConcurrentRouteOps tests concurrent mixed route operations
func BenchmarkConcurrentRouteOps(b *testing.B) {
	s := NewWSServer()

	var wg sync.WaitGroup
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()
		i := uint32(0)
		for pb.Next() {
			streamID := i % 5000
			key := routeKey("src", streamID)
			switch i % 4 {
			case 0:
				route := &RouteInfo{
					SourceClientID: "src",
					TargetClientID: "tgt",
					StreamID:       streamID,
				}
				s.routes.Store(key, route)
				s.routes.Store(routeKey("tgt", streamID), route)
			case 1:
				s.routes.Load(key)
			case 2:
				s.routes.LoadAndDelete(key)
			case 3:
				s.routes.Load(key)
			}
			i++
		}
	})

	wg.Wait()
}
