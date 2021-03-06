package sysdigtracer

import (
	"bytes"
	"math/rand"
	"os"
	"sync"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// var poolBS sync.Pool = sync.Pool{
// 	New: func() interface{} {
// 		return make([]byte, 1024)
// 	},
// }

var poolSpan sync.Pool = sync.Pool{
	New: func() interface{} {
		return &sysdigspan{}
	},
}

type sysdigspan struct {
	tagsa    [128]byte
	tags     []byte
	id       uint64
	tracer   *sysdigtracer
	finished bool
}

type sysdigtracer struct {
	mu   sync.Mutex
	f    *os.File
	obuf bytes.Buffer
}

var defaulttracer *sysdigtracer

func New() opentracing.Tracer {
	rv := &sysdigtracer{}
	f, err := os.OpenFile("/dev/null", os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	rv.f = f
	return rv
}

func (n *sysdigspan) ForeachBaggageItem(handler func(k, v string) bool) {}

var (
	seededIDGen = rand.New(rand.NewSource(time.Now().UnixNano()))
	// The golang rand generators are *not* intrinsically thread-safe.
	seededIDLock sync.Mutex
)

func randomID() uint64 {
	seededIDLock.Lock()
	defer seededIDLock.Unlock()
	return uint64(seededIDGen.Int63()) | 0x1000000000000000
}

type sysdigspancontext struct {
	id int64
}

// Create, start, and return a new Span with the given `operationName` and
// incorporate the given StartSpanOption `opts`. (Note that `opts` borrows
// from the "functional options" pattern, per
// http://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis)
//
// A Span with no SpanReference options (e.g., opentracing.ChildOf() or
// opentracing.FollowsFrom()) becomes the root of its own trace.
//
// Examples:
//
//     var tracer opentracing.Tracer = ...
//
//     // The root-span case:
//     sp := tracer.StartSpan("GetFeed")
//
//     // The vanilla child span case:
//     sp := tracer.StartSpan(
//         "GetFeed",
//         opentracing.ChildOf(parentSpan.Context()))
//
//     // All the bells and whistles:
//     sp := tracer.StartSpan(
//         "GetFeed",
//         opentracing.ChildOf(parentSpan.Context()),
//         opentracing.Tag("user_agent", loggedReq.UserAgent),
//         opentracing.StartTime(loggedReq.Timestamp),
//     )
//
func (sd *sysdigtracer) StartSpan(
	operationName string,
	optz ...opentracing.StartSpanOption,
) opentracing.Span {

	rv := poolSpan.Get().(*sysdigspan)
	rv.tracer = sd
	rv.finished = false
	rv.id = 0
	if rv.tags == nil {
		rv.tags = rv.tagsa[:0]
	} else {
		rv.tags = rv.tags[:0]
	}

	for _, o := range optz {
		rf, ok := o.(opentracing.SpanReference)
		if ok {
			refCtx := rf.ReferencedContext.(*sysdigspan)
			rv.tags = append(rv.tags, refCtx.tags...)
			rv.tags = append(rv.tags, '.')
			rv.id = refCtx.id
			break
		}
	}
	/*
		ReferencesLoop:
			for _, ref := range opts.References {
				switch ref.Type {
				case opentracing.ChildOfRef,
					opentracing.FollowsFromRef:

					refCtx := ref.ReferencedContext.(*sysdigspan)
					rv.tags = append(rv.tags, refCtx.tags...)
					rv.tags = append(rv.tags, '.')
					rv.id = refCtx.id
					break ReferencesLoop
				}
			}*/
	if rv.id == 0 {
		rv.id = randomID()
		//	fmt.Printf("random id was %d\n", rv.id)
	}
	for i := 0; i < len(operationName); i++ {
		rv.tags = append(rv.tags, byte(operationName[i]))
	}
	//rv.tags = append(rv.tags, []byte(operationName)...)
	//trbuf := poolBS.Get().([]byte)
	alen := 22 + len(rv.tags) + 2
	var trbuf []byte
	if alen < 256 {
		trbufa := [256]byte{}
		trbuf = trbufa[:alen]
	} else {
		trbuf = make([]byte, alen)
	}

	//	trbuf := make([]byte, 23+len(rv.tags)+2)
	trbuf[0] = '>'
	trbuf[1] = ':'
	id := rv.id
	for i := 20; i >= 2; i-- {
		trbuf[i] = '0' + byte(id%10)
		id /= 10
	}
	trbuf[21] = ':'
	copy(trbuf[22:], rv.tags)
	trbuf[22+len(rv.tags)] = ':'
	trbuf[23+len(rv.tags)] = ':'
	//	tr := fmt.Sprintf(">:%d:%s::\n", rv.id, rv.tags)
	//fmt.Print(tr)
	//fmt.Println(string(trbuf))
	sd.f.Write(trbuf)
	//poolBS.Put(trbuf)
	return rv
}

// noopSpan:
func (n *sysdigspan) Context() opentracing.SpanContext { return n }
func (n *sysdigspan) SetBaggageItem(key, val string) opentracing.Span {
	return n
}
func (n *sysdigspan) BaggageItem(key string) string { return "" }
func (n *sysdigspan) SetTag(key string, value interface{}) opentracing.Span {
	return n
}
func (n *sysdigspan) LogFields(fields ...log.Field) {}
func (n *sysdigspan) LogKV(keyVals ...interface{})  {}
func (n *sysdigspan) Finish() {
	if n.finished {
		return
	}
	n.finished = true
	trbufa := [128]byte{}
	trbuf := trbufa[:22+len(n.tags)+2]
	id := n.id
	//	trbuf = trbuf[:23+len(n.tags)+2]
	trbuf[0] = '<'
	trbuf[1] = ':'
	for i := 20; i >= 2; i-- {
		trbuf[i] = '0' + byte(id%10)
		id /= 10
	}
	trbuf[21] = ':'
	copy(trbuf[22:], n.tags)
	trbuf[22+len(n.tags)] = ':'
	trbuf[23+len(n.tags)] = ':'
	//	tr := fmt.Sprintf(">:%d:%s::\n", rv.id, rv.tags)
	//fmt.Print(tr)
	//	fmt.Println(string(trbuf))
	n.tracer.f.Write(trbuf)
	//	fmt.Println(string(trbuf))
	//	poolBS.Put(trbuf)

	// bs := poolBS.Get().([]byte)[:0]
	// b := bytes.NewBuffer(bs)
	// b.WriteString(fmt.Sprintf("<:%d:%s::\n", n.id, n.tags))
	// first := true
	// for k, v := range n.arguments {
	// 	if !first {
	// 		b.WriteString(",")
	// 	}
	// 	first = false
	// 	b.WriteString(k)
	// 	b.WriteString("=")
	// 	b.WriteString(v)
	// }
	// b.WriteString("\n")
	// //fmt.Print(b.String())
	// _, err := n.tracer.f.Write(b.Bytes())
	// if err != nil {
	// 	panic(err)
	// }
	// poolBS.Put(bs)
	//poolBS.Put(n.tags)
	poolSpan.Put(n)
}
func (n *sysdigspan) FinishWithOptions(opts opentracing.FinishOptions) {
	n.Finish()
}
func (n *sysdigspan) SetOperationName(operationName string) opentracing.Span {
	return n
}
func (n *sysdigspan) Tracer() opentracing.Tracer {
	return n.tracer
}
func (n *sysdigspan) LogEvent(event string)                                 {}
func (n *sysdigspan) LogEventWithPayload(event string, payload interface{}) {}
func (n *sysdigspan) Log(data opentracing.LogData)                          {}

func (sd *sysdigtracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	return nil
}

func (sd *sysdigtracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	return nil, opentracing.ErrSpanContextNotFound
}
