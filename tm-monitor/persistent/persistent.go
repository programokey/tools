package persistent

import "gopkg.in/mgo.v2"

var (
	mgoUrl = "localhost"
)

type Persistent struct{
	session 		*mgo.Session
	dbName 			string
	collectionName 	string
	collection 		*mgo.Collection
}

// initialize the mongo session
func NewPersistent(dbName, collectionName string) *Persistent {
	p := &Persistent{}
	p.dbName = dbName
	p.collectionName = collectionName
	var err error
	p.session, err = mgo.Dial(mgoUrl)
	p.session.SetMode(mgo.Monotonic, true)
	if err != nil {
		panic(err)
	}
	p.collection = p.session.DB(p.dbName).C(p.collectionName)
	return p
}

func (p *Persistent) Save(data interface{}) {
	p.collection.Insert(data)
}

func (p *Persistent) Find(query interface{}) (result []interface{}){
	p.collection.Find(query).All(&result)
	return result
}

func (p *Persistent) FindWithType(query interface{}, result []interface{}) ([]interface{}){
	p.collection.Find(query).All(&result)
	return result
}

func (p *Persistent) Index(keys []string) {
	p.collection.EnsureIndex(
		mgo.Index{
			Key: keys,
			Unique: true,
			DropDups: true,
			Background: true, // See notes.
			Sparse: true,
		})
}


func (p *Persistent) Close() {
	p.session.Close()
}


