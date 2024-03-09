package model

import (
	"fmt"
	"google.golang.org/appengine/v2/aetest"
	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
	"google.golang.org/appengine/v2/memcache"
	"reflect"
	"testing"
)

type Entity struct {
	Model
	Name       string
	Num        int
	Child      Child
	EmptyChild EmptyChild `model:"zero"`
	ReadonlyChild `model:"readonly"`
}

type Child struct {
	Model
	Name       string
	Grandchild Grandchild
}

type Grandchild struct {
	Model
	GrandchildNum int
}

type EmptyChild struct {
	Model
	Emptiness int
}

type ReadonlyChild struct {
	Model
	Value int
}

type ExtendedEntity struct {
	Model
	Name string
	Child Child
	Ext interface{}
	Pls *StructPLS
}

type ExtensionImpl struct {
	Name string
}

type StructPLS struct {
	PLSVal string
}

func (pls *StructPLS) Load(props []datastore.Property) error {
	for _, p := range props {
		switch p.Name {
		case "enzo":
			pls.PLSVal = p.Value.(string)
		}
	}
	return nil
}

func (pls *StructPLS) Save() ([]datastore.Property, error) {
	return []datastore.Property{
		{Name: "enzo", Value: pls.PLSVal},
	}, nil
}

const total = 100
const find = 10

func TestIndexing(t *testing.T) {

	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	// test correct indexing
	entity := Entity{}
	index(&entity)
	if !entity.EmptyChild.skipIfZero {
		t.Fatal("empty child is not skipIfZero")
	}

	entity.Name = "entity"
	entity.Child.Name = "child"
	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Read(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if entity.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key")
	}
}

func TestUpdate(t *testing.T) {
	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	rc := ReadonlyChild{}
	rc.Value = 1
	err = Create(ctx, &rc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test correct indexing
	entity := Entity{}
	entity.Child.Name = "child"
	entity.Child.Grandchild.GrandchildNum = 10
	entity.ReadonlyChild = rc
	entity.ReadonlyChild.Value = 100

	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Read(ctx, &entity)
	if err != nil {
		t.Fatal(err)
	}
	entity.Child.Grandchild.GrandchildNum = 100
	entity.Child.Name = ""

	if err = Read(ctx, &entity.ReadonlyChild); err != nil {
		t.Fatal(err.Error())
	}

	if entity.ReadonlyChild.Value != 1 {
		t.Fatal("readonly child has been updated")
	}

	err = Update(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if entity.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key after update")
	}

	if entity.Child.Grandchild.GrandchildNum != 100 {
		t.Fatalf("grand child has not been updated. Num is %d", entity.Child.Grandchild.GrandchildNum)
	}

	if entity.Child.Name != "" {
		t.Fatalf("child has not been updated. Name is %s", entity.Child.Name)
	}
}

func TestDelete(t *testing.T) {

	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	rc := ReadonlyChild{}
	err = Create(ctx, &rc)
	if err != nil {
		log.Errorf(ctx, err.Error())
	}

	err = memcache.Flush(ctx)
	if err != nil {
		log.Errorf(ctx, err.Error())
	}

	// test correct indexing
	entity := Entity{}
	entity.Name = "Enzo"
	entity.Child.Name = "child"
	entity.Child.Grandchild.GrandchildNum = 10
	entity.ReadonlyChild = rc

	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Delete(ctx, &(entity.Child), &entity)
	if err != nil {
		t.Fatalf(err.Error())
	}

	q := NewQuery((*Child)(nil))
	q = q.WithField("Name = ", "child")
	err = q.First(ctx, &entity.Child)
	if err == nil {
		t.Fatalf("child must have been deleted. Found child %+v", entity.Child)
	}

	t.Logf("can't find child: %s", err.Error())

	e := Entity{}
	q = NewQuery(&e)
	q = q.WithField("Name =", "Enzo")
	err = q.First(ctx, &e)
	if err != nil {
		t.Fatalf("can't find entity with name Delete: %s", err.Error())
	}
}

func TestModelQuery(t *testing.T) {

	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	for i := 0; i < total; i++ {
		entity := Entity{}
		entity.Name = fmt.Sprintf("%d", i)
		entity.Num = i
		entity.Child.Name = fmt.Sprintf("child-%s", entity.Name)
		entity.Child.Grandchild.GrandchildNum = i
		err := Create(ctx, &entity)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = memcache.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}

	dst := make([]*Entity, 0)

	q := NewQuery(&Entity{})
	q = q.WithField("Num >", find)
	err = q.GetMulti(ctx, &dst)
	if err != nil {
		t.Fatal(err)
	}

	if len(dst) != total - find - 1 {
		t.Fatalf("invalid number of data returned. Count is %d", len(dst))
	}

	for k := find + 1; k < total; k++ {
		idx := k - find - 1
		entity := dst[idx]
		if entity.Num != k {
			t.Fatalf("invalid error. entity at index %d has value %d.", idx, entity.Num)
		}
	}
}

// analogous to TestIndexing, but flushes memcache between write and read operation
func TestDatastoreModel(t *testing.T) {
	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	// test correct indexing
	entity := Entity{}
	index(&entity)
	if !entity.EmptyChild.skipIfZero {
		t.Fatal("empty child is not skipIfZero")
	}

	entity.Name = "entity"
	entity.Child.Name = "child"
	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err = memcache.Flush(ctx); err != nil {
		t.Fatalf("error flushing memcache: %s", err.Error())
	}

	e := Entity{}
	err = FromEncodedKey(ctx, &e, entity.EncodedKey())
	if err != nil {
		t.Fatal(err.Error())
	}

	if e.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key")
	}

	if e.Name != "entity" {
		t.Fatalf("Entity name has changed between write and read. Is %q must be \"entity\"", e.Name)
	}

	if e.Child.Name != "child" {
		t.Fatalf("Reference name has changed between write and read. Is %q must be \"child\"", e.Child.Name)
	}
}

func TestModelExtension(t *testing.T) {
	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	// test correct indexing
	entity := ExtendedEntity{}

	ext := &ExtensionImpl{Name: "extName"}

	entity.Ext = ext
	entity.Name = "entity"
	entity.Child.Name = "child"
	entity.Pls = &StructPLS{PLSVal:"plspls"}
	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	re := ExtendedEntity{}
	err = FromIntID(ctx, &re, entity.IntID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	if re.Name != "entity" {
		t.Fatalf("Entity name has changed between write and read. Is %q must be \"entity\"", re.Name)
	}

	if re.Child.Name != "child" {
		t.Fatalf("Reference name has changed between write and read. Is %q must be \"child\"", re.Child.Name)
	}

	ext = re.Ext.(*ExtensionImpl)
	if ext.Name != "extName" {
		t.Fatalf("Extension name has changed between write and read. Is %q must be \"extName\"", ext.Name)
	}

	if re.Pls.PLSVal != "plspls" {
		t.Fatalf("StructPLS name has changed between write and read. Is %q must be \"plspls\"", ext.Name)
	}
}

func TestModelExtensionCached(t *testing.T) {
	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	// test correct indexing
	entity := ExtendedEntity{}

	ext := &ExtensionImpl{Name: "extName"}

	entity.Ext = ext
	entity.Name = "entity"
	entity.Child.Name = "child"
	entity.Pls = &StructPLS{PLSVal:"plspls"}
	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err = memcache.Flush(ctx); err != nil {
		t.Fatalf("error flushing memcache: %s", err.Error())
	}

	re := ExtendedEntity{}
	err = FromIntID(ctx, &re, entity.IntID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	if re.Name != "entity" {
		t.Fatalf("Entity name has changed between write and read. Is %q must be \"entity\"", re.Name)
	}

	if re.Child.Name != "child" {
		t.Fatalf("Reference name has changed between write and read. Is %q must be \"child\"", re.Child.Name)
	}

	ext = re.Ext.(*ExtensionImpl)
	if ext.Name != "extName" {
		t.Fatalf("Extension name has changed between write and read. Is %q must be \"extName\"", ext.Name)
	}

	if re.Pls.PLSVal != "plspls" {
		t.Fatalf("StructPLS name has changed between write and read. Is %q must be \"plspls\"", ext.Name)
	}
}

func BenchmarkMapStructureLocked(b *testing.B) {
	entity := Entity{}
	typ := reflect.TypeOf(entity)
	structure := newEncodedStruct(typ.Name())
	for n := 0; n < b.N; n++ {
		mapStructureLocked(typ, structure)
	}
}

func BenchmarkIsEmpty(b *testing.B) {
	entity := Entity{}
	for n := 0; n < b.N; n++ {
		IsEmpty(&entity)
	}
}

func BenchmarkIndexing(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		entity := Entity{}
		index(&entity)
	}
}

func BenchmarkIndexingSimple(b *testing.B) {
	for n := 0; n < b.N; n++ {
		gc := Grandchild{}
		index(&gc)
	}
}

func BenchmarkReindexing(b *testing.B) {
	entity := Entity{}
	for n := 0; n < b.N; n++ {
		index(&entity)
	}
}
