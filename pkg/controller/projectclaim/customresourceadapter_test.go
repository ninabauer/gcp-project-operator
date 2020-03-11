package projectclaim_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	gcpv1alpha1 "github.com/openshift/gcp-project-operator/pkg/apis/gcp/v1alpha1"
	"github.com/openshift/gcp-project-operator/pkg/controller/projectclaim"
	. "github.com/openshift/gcp-project-operator/pkg/controller/projectclaim"
	"github.com/openshift/gcp-project-operator/pkg/util/mocks"
	testStructs "github.com/openshift/gcp-project-operator/pkg/util/mocks/structs"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var _ = Describe("Customresourceadapter", func() {
	var (
		adapter      *CustomResourceAdapter
		mockCtrl     *gomock.Controller
		mockClient   *mocks.MockClient
		projectClaim *gcpv1alpha1.ProjectClaim
	)

	BeforeEach(func() {
		projectClaim = testStructs.NewProjectClaimBuilder().GetProjectClaim()
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
	})
	JustBeforeEach(func() {
		adapter = NewCustomResourceAdapter(projectClaim, logf.Log.WithName("Test Logger"), mockClient)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("ProjectReferenceExists", func() {
		Context("when ProjectReference exists", func() {
			It("returns true", func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, *testStructs.NewProjectReferenceBuilder().GetProjectReference())
				exists, err := adapter.ProjectReferenceExists()
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})

		Context("when ProjectReference doesn't exist", func() {
			It("returns false", func() {
				notFound := errors.NewNotFound(schema.GroupResource{}, "FakeProjectReference")
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound)
				exists, err := adapter.ProjectReferenceExists()
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})
	})

	Context("IsProjectClaimDeletion", func() {
		It("returns true when DeletionTimeStamp is set on ProjectClaim", func() {
			deletionTime := metav1.NewTime(time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC))
			projectClaim.SetDeletionTimestamp(&deletionTime)
			Expect(adapter.IsProjectClaimDeletion()).To(BeTrue())
		})

		It("returns false when DeletionTimeStamp is not set on ProjectClaim", func() {
			projectClaim.SetDeletionTimestamp(nil)
			Expect(adapter.IsProjectClaimDeletion()).To(BeFalse())
		})
	})

	Context("FinalizeProjectClaim", func() {
		var (
			matcher *testStructs.ProjectClaimMatcher
		)
		BeforeEach(func() {
			projectClaim = testStructs.NewProjectClaimBuilder().WithFinalizer([]string{ProjectClaimFinalizer}).GetProjectClaim()
			matcher = testStructs.NewProjectClaimMatcher()
		})

		Context("when the project reference doesn't exist", func() {
			BeforeEach(func() {
				notFound := errors.NewNotFound(schema.GroupResource{}, "FakeProjectReference")
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound)
				mockClient.EXPECT().Update(gomock.Any(), matcher).Times(1)
			})

			It("removes the finalizer", func() {
				err := adapter.FinalizeProjectClaim()
				Expect(err).ToNot(HaveOccurred())
				Expect(matcher.ActualProjectClaim.Finalizers).ToNot(ContainElement(ProjectClaimFinalizer))
			})
		})

		Context("when the project reference exists", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, *testStructs.NewProjectReferenceBuilder().GetProjectReference())
			})

			It("deletes the ProjectReference and removes the finalizer", func() {
				mockClient.EXPECT().Delete(gomock.Any(), &testStructs.ProjectReferenceMatcher{}).Times(1)
				mockClient.EXPECT().Update(gomock.Any(), matcher).Times(1)
				err := adapter.FinalizeProjectClaim()
				Expect(err).ToNot(HaveOccurred())
				Expect(matcher.ActualProjectClaim.Finalizers).ToNot(ContainElement(ProjectClaimFinalizer))
			})

			Context("when deleting the project reference fails", func() {
				BeforeEach(func() {
					mockClient.EXPECT().Delete(gomock.Any(), &testStructs.ProjectReferenceMatcher{}).Return(fmt.Errorf("Fake Error"))
				})

				It("does not remove the finalizer", func() {
					mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)
					err := adapter.FinalizeProjectClaim()
					Expect(err).To(HaveOccurred())
					Expect(projectClaim.Finalizers).To(ContainElement(ProjectClaimFinalizer))
				})
			})
		})
	})

	Context("EnsureProjectReferenceLink", func() {
		Context("when ProjectReferenceCRLink is not set", func() {
			It("sets the ProjectReferenceCRLink and returns ObjectModified", func() {
				matcher := testStructs.NewProjectClaimMatcher()
				mockClient.EXPECT().Update(gomock.Any(), matcher).Times(1)
				crState, err := adapter.EnsureProjectReferenceLink()
				Expect(err).ToNot(HaveOccurred())
				Expect(matcher.ActualProjectClaim.Spec.ProjectReferenceCRLink.Name).To(Equal(projectClaim.GetNamespace() + "-" + projectClaim.GetName()))
				Expect(matcher.ActualProjectClaim.Spec.ProjectReferenceCRLink.Namespace).To(Equal(gcpv1alpha1.ProjectReferenceNamespace))
				Expect(crState).To(Equal(projectclaim.ObjectModified))
			})
		})

		Context("when ProjectReferenceCRLink is set", func() {
			BeforeEach(func() {
				projectClaim.Spec.ProjectReferenceCRLink.Name = projectClaim.GetNamespace() + "-" + projectClaim.GetName()
				projectClaim.Spec.ProjectReferenceCRLink.Namespace = gcpv1alpha1.ProjectReferenceNamespace
			})

			It("doesn't update the ProjectClaim", func() {
				crState, err := adapter.EnsureProjectReferenceLink()
				Expect(err).ToNot(HaveOccurred())
				Expect(crState).To(Equal(projectclaim.ObjectUnchanged))
			})
		})
	})

	Context("EnsureFinalizer", func() {
		Context("when finalizer is not set", func() {
			BeforeEach(func() {
				projectClaim.Finalizers = []string{}
			})
			It("sets the finalizer", func() {
				matcher := testStructs.NewProjectClaimMatcher()
				mockClient.EXPECT().Update(gomock.Any(), matcher).Times(1)
				crState, err := adapter.EnsureFinalizer()
				Expect(err).NotTo(HaveOccurred())
				Expect(crState).To(Equal(projectclaim.ObjectModified))
				Expect(projectClaim.Finalizers).To(ContainElement(projectclaim.ProjectClaimFinalizer))
			})
		})

		Context("when finalizer is set already", func() {
			BeforeEach(func() {
				projectClaim.Finalizers = []string{projectclaim.ProjectClaimFinalizer}
			})
			It("doesn't change the ProjectClaim", func() {
				crState, err := adapter.EnsureFinalizer()
				Expect(err).NotTo(HaveOccurred())
				Expect(crState).To(Equal(projectclaim.ObjectUnchanged))
			})
		})
	})

	Context("EnsureProjectReferenceExists()", func() {
		Context("when matching ProjectReference doesn't exist", func() {
			BeforeEach(func() {
				notFound := errors.NewNotFound(schema.GroupResource{}, "FakeProjectReference")
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound)
			})
			It("creates a ProjectReference", func() {
				matcher := testStructs.NewProjectReferenceMatcher()
				mockClient.EXPECT().Create(gomock.Any(), matcher)
				err := adapter.EnsureProjectReferenceExists()
				Expect(err).ToNot(HaveOccurred())
				Expect(matcher.ActualProjectReference.Name).To(Equal(projectClaim.GetNamespace() + "-" + projectClaim.GetName()))
				Expect(matcher.ActualProjectReference.Namespace).To(Equal(gcpv1alpha1.ProjectReferenceNamespace))
				Expect(matcher.ActualProjectReference.Spec.ProjectClaimCRLink.Name).To(Equal(projectClaim.Name))
				Expect(matcher.ActualProjectReference.Spec.ProjectClaimCRLink.Namespace).To(Equal(projectClaim.Namespace))
				Expect(matcher.ActualProjectReference.Spec.LegalEntity).To(Equal(projectClaim.Spec.LegalEntity))
			})
		})

		Context("when matching ProjectReference exists", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, *testStructs.NewProjectReferenceBuilder().GetProjectReference())
			})
			It("doesn't return an error", func() {
				err := adapter.EnsureProjectReferenceExists()
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("EnsureProjectClaimState()", func() {
		var (
			requestedState gcpv1alpha1.ClaimStatus
			currentState   gcpv1alpha1.ClaimStatus
		)
		JustBeforeEach(func() {
			projectClaim.Status.State = currentState
		})

		Context("when requested state is Pending", func() {
			BeforeEach(func() {
				requestedState = gcpv1alpha1.ClaimStatusPending
			})

			Context("when ProjectClaim state is not empty", func() {
				BeforeEach(func() {
					currentState = gcpv1alpha1.ClaimStatusReady
				})
				It("doesn't change the ProjectClaim state", func() {
					adapter.EnsureProjectClaimState(requestedState)
					Expect(projectClaim.Status.State).To(Equal(currentState))
				})
			})

			Context("when ProjectClaim state is empty", func() {
				BeforeEach(func() {
					currentState = ""
				})
				It("updates the state to Pending", func() {
					mockClient.EXPECT().Status().Times(1).Return(stubStatus{})
					adapter.EnsureProjectClaimState(requestedState)
					Expect(projectClaim.Status.State).To(Equal(requestedState))
				})
			})
		})

		Context("when requested state is PendingProject", func() {
			BeforeEach(func() {
				requestedState = gcpv1alpha1.ClaimStatusPendingProject
			})

			Context("when ProjectClaim state is not Pending", func() {
				BeforeEach(func() {
					currentState = gcpv1alpha1.ClaimStatusReady
				})
				It("doesn't change the ProjectClaim state", func() {
					adapter.EnsureProjectClaimState(requestedState)
					Expect(projectClaim.Status.State).To(Equal(currentState))
				})
			})

			Context("when ProjectClaim state is Pending", func() {
				BeforeEach(func() {
					currentState = gcpv1alpha1.ClaimStatusPending
				})
				It("updates the state to PendingProject", func() {
					mockClient.EXPECT().Status().Times(1).Return(stubStatus{})
					adapter.EnsureProjectClaimState(requestedState)
					Expect(projectClaim.Status.State).To(Equal(requestedState))
				})
			})
		})
	})
})

type stubStatus struct{}

func (stubStatus) Update(ctx context.Context, obj runtime.Object) error {
	return nil
}
