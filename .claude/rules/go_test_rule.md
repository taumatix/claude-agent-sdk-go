## Testing

### Well-known libraries

- *github.com/stretchr/testify/assert* is the preferred library for test assertions
- *github.com/stretchr/testify/require* must be used when an assertion is required to pass (i.e. `require.NotNil` to prevent nil pointer panics later in the test)

### Mocking / faking a dependency

Avoid complex mocking libraries. Prefer stand-alone interface implementation.

Example

```go
// in the go module
type MyInterface interface{
    DoSomething (arg1 type1) (type2, error)
}

// in a _test.go file
type MyInterfaceFuncs struct {
    DoSomethingFunc func(arg1 type1) (type2, error)
    DoSomethingCalls int
}

func (i *MyInterfaceFuncs) DoSomething(arg1 type1) (type2, error) {
    i.DoSomethingCalls++
    // If the interface can return an error
    if i.DoSomethingFunc == nil {
        return nil, errors.New("DoSomething not implemented")
    }
    return i.DoSomethingFunc(arg1)
}

// in the test
func TestMyFeature(t *testing.T) {
    myDependency := MyInterfaceFuncs{
        DoSomethingFunc: func(arg1 type1) (type2, error) {
            // do relevant asserts on arg1
            assert.NotNil(t, arg1)
            return [...], nil
        }
    }

    myFeature := MyFeature{Dependency: myDependecy}

    err := myFeature.CallTestedFunction()
    assert.NoError(t, err)

    assert.Equal(t, 1, myDependnecy.DoSomethingCalls)
}
```

### Test case readability

We favour test and test output readability. For this we prefer defining test helpers from table testing with nested loops in tests.

Example

```go

func testAdd1(t *testing.T, arg1 type1, expected type2) {
    t.Helper()
    doSomeInitialization()
    got := add1(arg1)
    require.NotNil(t, got)
    assert.Equal(t, expected, got)
}

func TestMyFunction(t *testing.T){
    testASpecificBehaviour(t, 1, 2)
    testASpecificBehaviour(t, 2, 3)
}

func testComplexAssert(t *testing.T, arg1 type 1, check func(t *testing.T, got type2)) {
    t.Helper()
    doSomeInitialization()
    got := doSomething(arg1)
    require.NotNil(t, got)
    check(got)
}



func TestMyComplexFunction(t *testing.T){
    testComplexAssert(t, 1, func(t *testing.T, got type2){assert.Greater(t, got, 1)})
    testComplexAssert(t, 2, func(t *testing.T, got type2){assert.Greater(t, got, 2)})
}
```
