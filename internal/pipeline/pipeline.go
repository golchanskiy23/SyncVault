package pipeline

import (
	"context"
	"fmt"
	"sync"
)

type Stage[I, O any] struct {
	fn func(ctx context.Context, input I) (O, error)
}

func NewStage[I, O any](fn func(ctx context.Context, input I) (O, error)) Stage[I, O] {
	return Stage[I, O]{fn: fn}
}

func (s Stage[I, O]) Process(ctx context.Context, input I) (O, error) {
	return s.fn(ctx, input)
}

type Pipeline[I, O any] struct {
	stages []func(context.Context, any) (any, error)
}

func New[I, O any]() *Pipeline[I, O] {
	return &Pipeline[I, O]{}
}

func Add[I, O any, N any](p *Pipeline[I, O], stage Stage[I, N]) *Pipeline[I, N] {
	universalFunc := func(ctx context.Context, input any) (any, error) {
		typedInput, ok := input.(I)
		if !ok {
			return nil, fmt.Errorf("type mismatch in pipeline stage")
		}
		return stage.Process(ctx, typedInput)
	}

	p.stages = append(p.stages, universalFunc)
	return &Pipeline[I, N]{stages: p.stages}
}

func (p *Pipeline[I, O]) Execute(ctx context.Context, input I) (O, error) {
	var current any = input

	for i, stage := range p.stages {
		select {
		case <-ctx.Done():
			var zero O
			return zero, ctx.Err()
		default:
			result, err := stage(ctx, current)
			if err != nil {
				var zero O
				return zero, err
			}
			current = result
			fmt.Printf("Stage %d: processed\n", i)
		}
	}

	output, ok := current.(O)
	if !ok {
		var zero O
		return zero, fmt.Errorf("final type mismatch in pipeline")
	}

	return output, nil
}

type ParallelPipeline[I, O any] struct {
	stages  []Stage[I, O]
	workers int
}

func NewParallel[I, O any](workers int) *ParallelPipeline[I, O] {
	return &ParallelPipeline[I, O]{workers: workers}
}

func (p *ParallelPipeline[I, O]) AddStage(stage Stage[I, O]) {
	p.stages = append(p.stages, stage)
}

func (p *ParallelPipeline[I, O]) ExecuteBatch(ctx context.Context, inputs []I) ([]O, error) {
	if len(p.stages) == 0 {
		return nil, nil
	}

	inputChan := make(chan I, len(inputs))
	resultChan := make(chan Result[O], len(inputs))

	go func() {
		defer close(inputChan)
		for _, input := range inputs {
			select {
			case <-ctx.Done():
				return
			case inputChan <- input:
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(p.workers)

	for i := 0; i < p.workers; i++ {
		go func(workerID int) {
			defer wg.Done()

			for input := range inputChan {
				var result O
				var err error

				currentInput := any(input)
				success := true

				for _, stage := range p.stages {
					select {
					case <-ctx.Done():
						return
					default:
						if typedInput, ok := currentInput.(I); ok {
							result, err = stage.Process(ctx, typedInput)
						} else {
							err = fmt.Errorf("type mismatch")
							success = false
							break
						}

						if err != nil {
							success = false
							break
						}
						currentInput = result
					}
				}

				if success {
					select {
					case <-ctx.Done():
						return
					case resultChan <- Result[O]{Value: result, Error: err}:
					}
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var results []O
	for res := range resultChan {
		results = append(results, res.Value)
	}

	return results, nil
}

type Result[T any] struct {
	Value T
	Error error
}
