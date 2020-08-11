package tlru

import (
	"sync"
	"testing"
	"time"
)

func TestCacheGetSet(t *testing.T) {
	c := NewCache(1, 0, time.Second)
	c.Set("a", true)
	r, ok := c.Get("a")
	if !ok {
		t.Fatal("should be ok")
		return
	}
	if r != true {
		t.Fatal("should be true")
	}
}

func TestCacheMemoryEviction(t *testing.T) {
	c := NewCache(10, 1, time.Millisecond*3)
	c.Set("a", true)
	_, ok := c.Get("a")
	if !ok {
		t.Fatal("should be ok")
	}
	c.Set("b", true)
	_, ok = c.Get("b")
	if !ok {
		t.Fatal("should be ok")
	}
	_, ok = c.Get("a")
	if ok {
		t.Fatal("should be not ok")
	}
	time.Sleep(time.Millisecond*5)
	if c.totalBytes != 0 {
		t.Fatal("total bytes hasn't reset")
	}
}

func TestCacheNoItemFound(t *testing.T) {
	c := NewCache(1, 0, time.Second)
	c.Set("a", true)
	_, ok := c.Get("b")
	if ok {
		t.Fatal("should be not ok")
	}
}

func TestCacheExpiry(t *testing.T) {
	c := NewCache(1, 0, time.Millisecond)
	c.Set("a", true)
	time.Sleep(time.Millisecond * 3)
	_, ok := c.Get("a")
	if ok {
		t.Fatal("should be not ok")
	}
}

func TestCacheFilling(t *testing.T) {
	c := NewCache(1, 0, time.Second)
	c.Set("a", true)
	c.Set("b", true)
	_, ok := c.Get("a")
	if ok {
		t.Fatal("should be not ok")
		return
	}
	r, ok := c.Get("b")
	if !ok {
		t.Fatal("should be ok")
		return
	}
	if r != true {
		t.Fatal("should be true")
	}
}

func TestCacheRaceConditionsGetSet(t *testing.T) {
	c := NewCache(2, 0, time.Second)
	for i := 0; i < 100000; i++ {
		go func(index int) {
			c.Set(1, 1)
			c.Set(index, 1)
			c.Get(1)
		}(i)
	}
}

func BenchmarkCache_Get50000Eviction(b *testing.B) {
	c := NewCache(10000, 0, time.Second*5)
	wg := sync.WaitGroup{}
	wg.Add(9999)
	for i := 0; i < 9999; i++ {
		go func(index int) {
			c.Set(index, 1)
			wg.Done()
		}(i)
	}
	wg.Wait()
	c.Set("a", 1)
	b.ResetTimer()
	wg.Add(50000)
	for i := 0; i < 50000; i++ {
		go func(index int) {
			defer wg.Done()
			c.Get("a")
		}(i)
	}
	wg.Wait()
}

func BenchmarkCache_SetIdeal(b *testing.B) {
	c := NewCache(10000, 0, time.Second)
	wg := sync.WaitGroup{}
	wg.Add(9999)
	for i := 0; i < 9999; i++ {
		go func(index int) {
			defer wg.Done()
			c.Set(index, 1)
		}(i)
	}
	wg.Wait()
	b.ResetTimer()
	c.Set("a", 1)
}

func BenchmarkCache_SetUnideal(b *testing.B) {
	c := NewCache(10000, 0, time.Second)
	wg := sync.WaitGroup{}
	wg.Add(5000)
	for i := 0; i < 5000; i++ {
		go func(index int) {
			defer wg.Done()
			c.Set(index, 1)
		}(i)
	}
	wg.Wait()
	c.Set("a", 1)
	wg.Add(4999)
	for i := 0; i < 4999; i++ {
		go func(index int) {
			defer wg.Done()
			c.Set(index, 1)
		}(i)
	}
	wg.Wait()
	b.ResetTimer()
	c.Set("a", 1)
}
