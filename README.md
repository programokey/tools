# tools
Tools for working with tendermint and associated technologies. See the documentation at: http://tendermint.readthedocs.io/en/master/index.html#tendermint-tools
# 使用tm-monitor和MongoDB记录区块数据

## 安装Mongo数据库
以下说明均是基于Ubuntu系统14.0及以后版本
1. 执行

    sudo apt-get install mongo
2. 因为Mongo数据库在运行时需要消耗大量的硬盘空间来存储数据，所以我们最好对MongoDB的数据默认存储路径进行修改。我们的服务器的/mnt目录挂载了一个容量比较大的硬盘，所以我们将数据存储路径修改到这个目录下。

    sudo mkdir /mnt/mongodb
    sudo vim /etc/mongodb.conf
将dbpath修改为

    dbpath=/var/lib/mongodb
3. 启动mongoDB
    
    sudo service mongodb start

4. 使用mongo命令进入Mongo数据库

## 编译安装tm-monitor
在编译安装tm-monitor的时候需要安装go-1.10以上版本的go，并且正确的设置GOPATH和GOROOT环境变量。这里假设已经安装完成。
1. 首先下载能够向MongoDB存储区块的tm-monitor


    go get -u github.com/programokey/tools
2. 下载完成后进入tm-monitor目录
    
    cd $GOPATH/src/github.com/programokey/tools/tm-monitor/
3. 运行
    
    make get_tools
    make get_vendor_deps
来获取必要的依赖包
4. 运行
    
    make install
编译安装tm-monitor

5. 使用以下命令运行tm-monitor

    tm-monitor -listen-addr=[listen-addr]  [endpoints]
这里的listen-addr是tm-monitor提供的查询接口监听的**本机地址**，endpoints是tm-monitor**要监控的Cosmos节点的地址和端口**，可以同时监听多个endpoints

    tm-monitor -listen-addr="tcp://0.0.0.0:46670" 54.215.221.213:46657

### 说明：
tm-monitor目前默认把block数据存在mongodb的tm-monitor数据库的block集合（collection)中，如果想修改存储的mongo数据库和集合，需要修改tm-monitor/monitor/node.go文件中的第82行：

    n.persistent = persistent.NewPersistent("tm-monitor", "block")
将这一行中的"tm-monitor", "block"分别换成你想要的db和collections。修改完成后使用make install重新编译即可。

