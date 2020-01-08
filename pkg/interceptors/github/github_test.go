package github

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/tektoncd/pipeline/pkg/logging"
	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	rtesting "knative.dev/pkg/reconciler/testing"
)

func TestInterceptor_ExecuteTrigger_Signature(t *testing.T) {
	type args struct {
		payload   []byte
		secret    *corev1.Secret
		signature string
		eventType string
	}
	tests := []struct {
		name    string
		GitHub  *triggersv1.GitHubInterceptor
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name:   "no secret",
			GitHub: &triggersv1.GitHubInterceptor{},
			args: args{
				payload:   []byte("somepayload"),
				signature: "foo",
			},
			want:    []byte("somepayload"),
			wantErr: false,
		},
		{
			name: "invalid header for secret",
			GitHub: &triggersv1.GitHubInterceptor{
				SecretRef: &triggersv1.SecretRef{
					SecretName: "mysecret",
					SecretKey:  "token",
				},
			},
			args: args{
				signature: "foo",
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mysecret",
					},
					Data: map[string][]byte{
						"token": []byte("secrettoken"),
					},
				},
				payload: []byte("somepayload"),
			},
			wantErr: true,
		},
		{
			name: "valid header for secret",
			GitHub: &triggersv1.GitHubInterceptor{
				SecretRef: &triggersv1.SecretRef{
					SecretName: "mysecret",
					SecretKey:  "token",
				},
			},
			args: args{
				// This was generated by using SHA1 and hmac from go stdlib on secret and payload.
				// https://play.golang.org/p/otp1o_cJTd7 for a sample.
				signature: "sha1=38e005ef7dd3faee13204505532011257023654e",
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mysecret",
					},
					Data: map[string][]byte{
						"token": []byte("secret"),
					},
				},
				payload: []byte("somepayload"),
			},
			wantErr: false,
			want:    []byte("somepayload"),
		},
		{
			name: "no secret, matching event",
			GitHub: &triggersv1.GitHubInterceptor{
				EventTypes: []string{"MY_EVENT", "YOUR_EVENT"},
			},
			args: args{
				payload:   []byte("somepayload"),
				eventType: "YOUR_EVENT",
			},
			wantErr: false,
			want:    []byte("somepayload"),
		},
		{
			name: "no secret, failing event",
			GitHub: &triggersv1.GitHubInterceptor{
				EventTypes: []string{"MY_EVENT", "YOUR_EVENT"},
			},
			args: args{
				payload:   []byte("somepayload"),
				eventType: "OTHER_EVENT",
			},
			wantErr: true,
		},
		{
			name: "valid header for secret and matching event",
			GitHub: &triggersv1.GitHubInterceptor{
				SecretRef: &triggersv1.SecretRef{
					SecretName: "mysecret",
					SecretKey:  "token",
				},
				EventTypes: []string{"MY_EVENT", "YOUR_EVENT"},
			},
			args: args{
				// This was generated by using SHA1 and hmac from go stdlib on secret and payload.
				// https://play.golang.org/p/otp1o_cJTd7 for a sample.
				signature: "sha1=38e005ef7dd3faee13204505532011257023654e",
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mysecret",
					},
					Data: map[string][]byte{
						"token": []byte("secret"),
					},
				},
				eventType: "MY_EVENT",
				payload:   []byte("somepayload"),
			},
			wantErr: false,
			want:    []byte("somepayload"),
		},
		{
			name: "valid header for secret, failing event",
			GitHub: &triggersv1.GitHubInterceptor{
				SecretRef: &triggersv1.SecretRef{
					SecretName: "mysecret",
					SecretKey:  "token",
				},
				EventTypes: []string{"MY_EVENT", "YOUR_EVENT"},
			},
			args: args{
				// This was generated by using SHA1 and hmac from go stdlib on secret and payload.
				// https://play.golang.org/p/otp1o_cJTd7 for a sample.
				signature: "sha1=38e005ef7dd3faee13204505532011257023654e",
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mysecret",
					},
					Data: map[string][]byte{
						"token": []byte("secret"),
					},
				},
				eventType: "OTHER_EVENT",
				payload:   []byte("somepayload"),
			},
			wantErr: true,
		},
		{
			name: "invalid header for secret, matching event",
			GitHub: &triggersv1.GitHubInterceptor{
				SecretRef: &triggersv1.SecretRef{
					SecretName: "mysecret",
					SecretKey:  "token",
				},
				EventTypes: []string{"MY_EVENT", "YOUR_EVENT"},
			},
			args: args{
				signature: "foo",
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mysecret",
					},
					Data: map[string][]byte{
						"token": []byte("secrettoken"),
					},
				},
				eventType: "MY_EVENT",
				payload:   []byte("somepayload"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := rtesting.SetupFakeContext(t)
			logger, _ := logging.NewLogger("", "")
			kubeClient := fakekubeclient.Get(ctx)
			request := &http.Request{
				Body: ioutil.NopCloser(bytes.NewReader(tt.args.payload)),
				GetBody: func() (io.ReadCloser, error) {
					return ioutil.NopCloser(bytes.NewReader(tt.args.payload)), nil
				},
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
			}
			if tt.args.eventType != "" {
				request.Header.Add("X-GITHUB-EVENT", tt.args.eventType)
			}
			if tt.args.signature != "" {
				request.Header.Add("X-Hub-Signature", tt.args.signature)
			}
			if tt.args.secret != nil {
				ns := tt.GitHub.SecretRef.Namespace
				if ns == "" {
					ns = metav1.NamespaceDefault
				}
				if _, err := kubeClient.CoreV1().Secrets(ns).Create(tt.args.secret); err != nil {
					t.Error(err)
				}
			}
			w := &Interceptor{
				KubeClientSet: kubeClient,
				GitHub:        tt.GitHub,
				Logger:        logger,
			}
			resp, err := w.ExecuteTrigger(request)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("Interceptor.ExecuteTrigger() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			got, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("error reading response body %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Interceptor.ExecuteTrigger() = %v, want %v", got, tt.want)
			}
		})
	}
}
