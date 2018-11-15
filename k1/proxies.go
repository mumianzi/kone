//
//   date  : 2016-05-13
//   author: xjdrew
//

package k1

import (
	"errors"
	"fmt"
	"github.com/xjdrew/proxy"
	"math/rand"
	"net"
	"strings"
	"time"
)

const (
	autoRandProxy  = "rand"
	autoSpeedProxy = "speed"
)

var errNoProxy = errors.New("no proxy")

var autoProxyType = []string{autoRandProxy, autoSpeedProxy}

type Proxies struct {
	proxies map[string]*proxy.Proxy
	dft     string
}

type TickerAutoProxy struct {
	Proxy    map[string]*proxy.Proxy
	Config   map[string]*ProxyConfig
	interval time.Duration
}

func (p *Proxies) Dial(proxy string, addr string) (net.Conn, error) {
	if proxy == "" {
		return p.DefaultDial(addr)
	}

	dialer := p.proxies[proxy]
	if dialer != nil {
		return dialer.Dial("tcp", addr)
	}
	return nil, fmt.Errorf("Invalid proxy: %s", proxy)
}

func (p *Proxies) RandAutoProxy(proxyName string, config map[string]*ProxyConfig) (bool) {

	url := generateProxyUrl(config[proxyName].Url)

	oriUrl := p.proxies[proxyName].Url.Scheme + "://" + p.proxies[proxyName].Url.Host

	if url == oriUrl {
		logger.Debugf("[proxies-%s] 代理无变化： %s ", proxyName, oriUrl)
		return false
	}

	proxy, err := proxy.FromUrl(url)

	if err != nil {
		return false
	}
	logger.Debugf("[proxies-%s] 当前代理是： %s ，新的代理为：%s", proxyName, oriUrl, url)

	config[proxyName].LastRefreshTime = time.Now()
	p.proxies[proxyName] = proxy

	return true
}

func (p *Proxies) SpeedAutoProxy(proxyName string, config map[string]*ProxyConfig) (bool) {
	//
	//if proxyName == "A"{
	//	time.Sleep(time.Second*2)
	//}
	//if proxyName == "home"{
	//	time.Sleep(time.Second*4)
	//}
	url := generateProxyUrl(config[proxyName].Url)

	for _, u := range config[proxyName].Url {
		go func(tUrl string) bool {
			p, err := proxy.FromUrl(tUrl)
			if err != nil {
				return false
			}

			return p
		}(u)
	}

	oriUrl := p.proxies[proxyName].Url.Scheme + "://" + p.proxies[proxyName].Url.Host

	if url == oriUrl {
		logger.Debugf("[proxies-%s] 代理无变化： %s ", proxyName, oriUrl)
		return false
	}

	proxy, err := proxy.FromUrl(url)

	if err != nil {
		return false
	}
	logger.Debugf("[proxies-%s] 当前代理是： %s ，新的代理为：%s", proxyName, oriUrl, url)

	config[proxyName].LastRefreshTime = time.Now()
	p.proxies[proxyName] = proxy

	return true
}

//自动选择代理定时器
func (p *Proxies) AutoSelectTimer(autos *TickerAutoProxy) {
	if l := len(autos.Proxy); l <= 0 {
		return
	}
	autoProxies := autos.Proxy

	go func(autoProxies map[string]*proxy.Proxy, autos *TickerAutoProxy) {

		tk := time.NewTicker(time.Second * autos.interval) //检查计时器
		for t := range tk.C {
			for name := range autoProxies {
				go func(name string, autos *TickerAutoProxy) {
					if name == "" {
						return
					}
					proxyConfig := autos.Config[name]
					autoType := strings.ToLower(proxyConfig.Auto)
					lastRefresh := proxyConfig.LastRefreshTime

					logger.Debugf("[proxies] 正在检查代理【%s】 %s,自动代理类型：%s，上次刷新时间：%s", name, t.Format("2006-01-02 15:04:05"), autoType, lastRefresh)

					if !p.checkInterval(lastRefresh, proxyConfig.Interval) {
						logger.Debugf("[proxies] 代理【%s】上次刷新时间【%s】过短，跳过", name, lastRefresh.Format("2006-01-02 15:04:05"))

						return
					}

					if autoType == autoSpeedProxy {
						p.SpeedAutoProxy(name, autos.Config)
					} else if autoType == autoRandProxy {
						p.RandAutoProxy(name, autos.Config)
					}

				}(name, autos)
			}
		}
	}(autoProxies, autos)
}

func (p *Proxies) checkInterval(lastRefresh time.Time, pInterval int64) bool {
	if pInterval <= 0 {
		pInterval = 300
	}

	if int64(time.Now().Sub(lastRefresh).Seconds()) < pInterval {
		return false
	}
	return true
}

func (p *Proxies) DefaultDial(addr string) (net.Conn, error) {
	dialer := p.proxies[p.dft]
	if dialer == nil {
		return nil, errNoProxy
	}
	return dialer.Dial("tcp", addr)
}

func NewProxies(one *One, config map[string]*ProxyConfig) (*Proxies, error) {
	p := &Proxies{}
	autoProxy := &TickerAutoProxy{}

	proxies := make(map[string]*proxy.Proxy)
	autos := make(map[string]*proxy.Proxy)

	for name, item := range config {

		//不管是不是自动测速的代理，先随机一个初始化
		proxy, err := proxy.FromUrl(generateProxyUrl(item.Url))

		if err != nil {
			return nil, err
		}

		if item.Default || p.dft == "" {
			p.dft = name
		}

		for v := range autoProxyType {
			if autoProxyType[v] == strings.ToLower(item.Auto) {
				logger.Debugf("[proxies] this is auto proxy: %s", name)
				autos[name] = proxy
				config[name].LastRefreshTime = time.Now()
			}
		}

		proxies[name] = proxy

		// don't hijack proxy domain
		host := proxy.Url.Host
		index := strings.IndexByte(proxy.Url.Host, ':')
		if index > 0 {
			host = proxy.Url.Host[:index]
		}
		one.rule.DirectDomain(host)
	}

	p.proxies = proxies
	logger.Infof("[proxies] default proxy: %q", p.dft)

	autoProxy.Proxy = autos
	autoProxy.Config = config
	autoProxy.interval = one.tickerInterval

	p.AutoSelectTimer(autoProxy)
	one.tickerProxies = autoProxy

	return p, nil
}

//随机一个代理
func generateProxyUrl(urls []string) string {

	if len(urls) <= 1 {
		return urls[0]
	}
	rand.Seed(time.Now().Unix())
	randNum := rand.Intn(len(urls))

	logger.Debugf("random Url is %s", urls[randNum])

	return urls[randNum]
}

//代理测速
func testProxySpeed(proxy proxy.Proxy) proxy.Proxy {

	return proxy
}
