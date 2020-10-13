
type ResponseWriter struct {

}


// Transition 1
func Transition(s State, e Event) State {
	if s == OnError {
		if e == WriteError {
			return PanicHard
		}
	}
}


// multiple
type transition func(state, event)(state, transition)

func beforeTransition func(state, event)(state, transition){
	if state.notwritten{
		return handleTransition
	}
	return writtenInBefor
}

type internalState struct {
	AreWeInsideAWrite bool
	WasAnythingWritten bool
	InternalError error
}

type before	 struct{
	internalState internalState
}

func (b before) Transition(e Event) State {
	return handle{}
}

var s stage
for condion {
	s =Transition(s, e)
}

var s stage
t:= starting
for t!=nil{
	s,t =t(s, e)
}

type Stage int

const (
	Initial Stage = iota
	Before
	Handler
	Commit
	After
	Done
)

Before -> ResponseWriter.Write -> Commit -> ResponseWriter.Write // possible scenario

type Stage interface {
	StageName() string
	Apply(State, Event) State
}

func (myState) Apply(s State, e Event) State {

}

type State struct {
	AreWeInsideAWrite bool
	WasAnythingWritten bool
	InternalError error
}
