/*
* @Author: HuberyChang
* @Date: 2021/4/29 17:34
 */

package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type User struct {
	name string
	id   string
	msg  chan string
}

// 定义一个全局map结构，用于保存所有的用户
var allUsers = make(map[string]User)

// 定义一个message全局通道，用于接收任何人发过来的消息
var message = make(chan string, 10)

func main() {
	// 创建服务器
	listener, err := net.Listen("tcp", ":8080")

	if err != nil {
		fmt.Println("net.Listen err:", err)
		return
	}

	// 启动唯一go程，负责监听message通道，写给所有的用户
	go broadcast()

	fmt.Println("服务器启动成功!")

	for {
		fmt.Println("==========>主go进程监听中...")

		// 监听
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("listener.Accept err:", err)
			return
		}

		// 建立连接
		fmt.Println("建立连接成功")

		// 启动处理业务的go程序
		go handler(conn)
	}

}

// 处理具体业务
func handler(conn net.Conn) {
	fmt.Println("启动业务...")

	// 客户端与服务器建立连接的时候，会有IP和port == 》 当成user的id
	clientAddr := conn.RemoteAddr().String()
	fmt.Println("clientAddr:", clientAddr)

	user := User{
		name: clientAddr,            // id作为map中的key，不修改
		id:   clientAddr,            // 可以改，会提供rename命令修改，建立连接超时,初始值与id相同
		msg:  make(chan string, 10), // 注意分配空间，否则无法写入数据
	}

	// 添加user到map
	allUsers[user.id] = user

	// 定义一个全局的退出信号，用于监听client退出
	var isQuit = make(chan bool)

	// 创建一个用于重置计数器的管道，用于告知watch函数，当前用户正在输入
	var resetTimer = make(chan bool)

	// 启动go程，负责监听退出的信号
	go watch(&user, conn, isQuit, resetTimer)

	// 启动go程，负责将msg信息返回给客户端
	go writeBackToClient(&user, conn)

	// 向message写入数据，当前用户上线的消息，用于通知所有人（广播）
	loginInfo := fmt.Sprintf("[%s]:[%s] ===== > 上线了！", user.id, user.name)
	message <- loginInfo

	for {
		// 具体业务逻辑
		buf := make([]byte, 1024)

		// 读取客户端发送过来的请求数据
		cnt, err := conn.Read(buf)
		if cnt == 0 {
			fmt.Println("客户端主动Ctrl C，准备退出,err:", err)
			// map删除用户，conn close掉
			// 服务器还可以主动退出
			// 在这里进行只真正的退出动作，而是发送一个退出信号，统一做退出处理，可以使用新得管道来做信号传递
			isQuit <- true
		}

		if err != nil {
			fmt.Println("conn.Read err:", err)
			return
		}

		fmt.Println("服务器接收客户端发送过来的数据为：", string(buf[:cnt-1]), ",cnt", cnt)

		// ------------------业务逻辑处理 开始 -------------------
		// 查询当前所有的用户 who
		// a.先判断接收的数据是不是who ===》 长度&&字符串
		userInput := string(buf[:cnt-1]) // 用户输入的数据，最后一个是回车，我们去掉它
		if len(userInput) == 4 && userInput == "\\who" {
			// b.遍历allUsers这个map：（key:=userID value: user本身），将id和name拼接成一个字符串，返回给客户端
			fmt.Println("用户查询所有用户：")

			// 切片包含所有的用户信息
			var userInfos []string

			for _, user := range allUsers {
				userInfo := fmt.Sprintf("userid:%s,username:%s", user.id, user.name)
				userInfos = append(userInfos, userInfo)
			}

			// 最终写到管道中， 一定是一个字符串
			r := strings.Join(userInfos, "\n")

			// 将数据返回给查询客户端
			user.msg <- r

		} else if len(userInput) > 9 && userInput[:7] == "\\rename" {
			user.name = strings.Split(userInput, "|")[1]
			allUsers[user.id] = user
			user.msg <- "rename successfully!"

		} else {
			// 如果用户输入的不是命令，只是普通信息，只需要写入到广播通道即可，有其他的go程处理
			message <- userInput
		}

		resetTimer <- true
		// ------------------业务逻辑处理 结束 -------------------

	}

}

// 启动一个全局唯一的go程
func broadcast() {
	fmt.Println("广播go程启动成功....")
	defer fmt.Println("broadcast程序退出")

	for {
		// 1、从message中读取数据
		fmt.Println("broadcast监听message中...")
		info := <-message

		fmt.Println("message 接收到消息：", info)

		// 2、将数据写入到每一个用户的msg管道中
		for _, user := range allUsers {
			// 如果msg是非缓冲的，会阻塞
			user.msg <- info
		}
	}
}

// 每一个用户应该还有一个用来监听自己msg管道的go程，负责将数据返回给客户端
func writeBackToClient(user *User, conn net.Conn) {
	fmt.Printf("1111111111111user:%s 正在监听自己的msg数据\n", user.name)
	for data := range user.msg {
		fmt.Printf("user:%s 写回给客户端的数据为：%s \n", user.name, data)
		_, _ = conn.Write([]byte(data))
	}
}

// 启动一个新的go程，负责监听退出信号，触发后，进行清零工作：delete map ，close conn 都在这里处理
func watch(user *User, conn net.Conn, isQuit, resetTimer <-chan bool) {
	fmt.Println("22222222222222启动监听退出信号的go程。。。")
	defer fmt.Println("watch go程退出")
	for {
		select {
		case <-isQuit:
			logoutInfo := fmt.Sprintf("%s exit", user.name)
			fmt.Println("删除当前用户：", user.name)
			delete(allUsers, user.id)
			message <- logoutInfo
			conn.Close()
			return
		case <-time.After(10 * time.Second):
			logoutInfo := fmt.Sprintf("%s timeout exit", user.name)
			fmt.Println("删除当前用户：", user.name)
			delete(allUsers, user.id)
			message <- logoutInfo
			conn.Close()
			return
		case <-resetTimer:
			fmt.Printf("连接%s 重置计数器！", user.name)

		}
	}
}
