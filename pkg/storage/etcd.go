package storage

import (
	"fmt"

	"github.com/operator-framework/catalogd/api/optional/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

type GenericStorage struct {
	*registry.Store
}

// NewRESTStorage returns a RESTStorage object that will work against API services.
func NewRESTStorage(scheme *runtime.Scheme, optsGetter generic.RESTOptionsGetter) (*GenericStorage, error) {
	strategy := NewStrategy(scheme)
	fmt.Println("Done creating strategy")
	store := &genericregistry.Store{
		NewFunc:                  func() runtime.Object { return &v1alpha1.CatalogMetadata{} },
		NewListFunc:              func() runtime.Object { return &v1alpha1.CatalogMetadataList{} },
		PredicateFunc:            MatchCatalogMetadata,
		DefaultQualifiedResource: v1alpha1.Resource("catalogmetadatas"),
		TableConvertor:           rest.NewDefaultTableConvertor(v1alpha1.Resource("catalogmetadatas")),

		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,
	}
	fmt.Println("About to create options")
	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}
	fmt.Println("About to return generic storage")
	return &GenericStorage{store}, nil
}
