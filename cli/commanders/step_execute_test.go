package commanders_test

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"

	. "github.com/onsi/gomega"
)

func newExecuteMessage(chunk *idl.Chunk) *idl.ExecuteMessage {
	return &idl.ExecuteMessage{
		Contents: &idl.ExecuteMessage_Chunk{chunk},
	}
}

func TestExecute(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("writes STDOUT and STDERR chunks in the order they are received", func(t *testing.T) {
		g := NewGomegaWithT(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_idl.NewMockCliToHubClient(ctrl)
		clientStream := mock_idl.NewMockCliToHub_ExecuteClient(ctrl)

		client.EXPECT().Execute(
			gomock.Any(),
			&idl.ExecuteRequest{},
		).Return(clientStream, nil)
		gomock.InOrder(
			clientStream.EXPECT().Recv().Return(newExecuteMessage(&idl.Chunk{
				Buffer: []byte("my string1"),
				Type:   idl.Chunk_STDOUT,
			}), nil),
			clientStream.EXPECT().Recv().Return(newExecuteMessage(&idl.Chunk{
				Buffer: []byte("my error"),
				Type:   idl.Chunk_STDERR,
			}), nil),
			clientStream.EXPECT().Recv().Return(newExecuteMessage(&idl.Chunk{
				Buffer: []byte("my string2"),
				Type:   idl.Chunk_STDOUT,
			}), nil),
			clientStream.EXPECT().Recv().Return(nil, io.EOF),
		)

		rOut, wOut, _ := os.Pipe()
		rErr, wErr, _ := os.Pipe()
		tmpOut := os.Stdout
		tmpErr := os.Stderr
		defer func() {
			os.Stdout = tmpOut
			os.Stderr = tmpErr
		}()
		os.Stdout = wOut
		os.Stderr = wErr
		go func() {
			defer wOut.Close()
			defer wErr.Close()
			err := commanders.Execute(client)
			g.Expect(err).To(BeNil())
		}()
		actualOut, _ := ioutil.ReadAll(rOut)
		actualErr, _ := ioutil.ReadAll(rErr)

		g.Expect(string(actualOut)).To(Equal("my string1my string2"))
		g.Expect(string(actualErr)).To(Equal("my error"))
	})

	t.Run("returns an error when a non io.EOF error is encountered", func(t *testing.T) {
		g := NewGomegaWithT(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_idl.NewMockCliToHubClient(ctrl)
		clientStream := mock_idl.NewMockCliToHub_ExecuteClient(ctrl)

		client.EXPECT().Execute(
			gomock.Any(),
			&idl.ExecuteRequest{},
		).Return(clientStream, nil)
		clientStream.EXPECT().Recv().Return(nil, io.ErrUnexpectedEOF)

		err := commanders.Execute(client)
		g.Expect(err).To(Equal(io.ErrUnexpectedEOF))
	})

	t.Run("it returns the error received from the hub", func(t *testing.T) {
		var client *mock_idl.MockCliToHubClient
		var ctrl *gomock.Controller
		ctrl = gomock.NewController(t)
		defer ctrl.Finish()
		clientStream := mock_idl.NewMockCliToHub_ExecuteClient(ctrl)

		expectedErr := errors.New("i failed for some reason")
		client = mock_idl.NewMockCliToHubClient(ctrl)
		client.EXPECT().Execute(
			gomock.Any(),
			&idl.ExecuteRequest{},
		).Return(clientStream, expectedErr).Times(1)

		err := commanders.Execute(client)

		if err != expectedErr {
			t.Errorf("got error: %#v wanted: %#v", err, expectedErr)
		}
	})

}
