# Defination of concept
Generally, there are three types of test cases:

- unit test；
- integration test , with mocking test；
- e2e test

each of them brings us different gains and pains. we will discuss each of them.
## Unit Test
The main purposes of writing and maintaining unit tests are as follows:

- Explicit: ensure that the function behavior meets the design expectations. 
- Implicit: facilitates more reasonable code structure and interface-oriented programming.

Test target of a unit test is a class or a segment of algorithm code. In theory, functions with relatively independent logic should has at least one unit test case, and new public functions must too (except for getter and setter).

Basic principles for writing a single test are:

- Keep It Simple and Stupid
- One case covers only one scenario.
- Complex mock tools are powerful, such as golang/mock, are not recommended. When you think you have to introduce mocking to unit test, what you really need is Integration test or event e2e test。

Single test requires high efficiency. For example, during code refactoring, after a function logic is modified, unittest of the entire module may be triggered to ensure that the modification meets expectations.

## Integration Test
The propose of integration test is to setup parts of highly associated modules in a system, test is the interaction between these modules meets expectations. In kubernetes product or distribution tests, it is mainly divided into two categories:


- The master component which contains : apiserver、 controllers、webooks, and schedulers. You need to add corresponding tests to major processes such as scale-out, release, and scheduling. With the mock sigmalet capability, you need to add a test coverage that includes the Automated Logic after the sigmalet exception.
- slave component, including the kubelet、cri implementations、daemonsets. you can add corresponding tests like creating, destroying, and restarting a pod or a crd.

The master level integration test does not care how pods are created on the node. Therefore, it can mock node level behavior to speed up the test efficiency. 
The slave level integration test does not care how pods are scheduled, and therefore doesn't need to setup master component.

## E2E Test
The propose of e2e test is to simulate The user's real behavior, suitable for verification of the whole product.

we recommend e2e test to be added in the following situations:

-  To interact with upstream and downstream products, for example:
   1. Some kubelet tests need to verify the attributes of the final allocated container from the docker level.
   1. Some controller test needs to interact with a service outside of the cluster, like a cloud service.
-  Core feature: each core feature must has at least one e2e test.

# Best Practice
The purpose of the test is to ensure the quality of continuous software delivery, with emphasis on the word "continuous". It is necessary to ensure not only the quality of the current delivery, but also the quality of future software delivery. It is particularly important to make good use of the respective advantages of the three types of tests and combine them to ensure the overall quality of the software.

|  | Time consumed Running | Test Stability | Can simulate User behavior |
| --- | --- | --- | --- |
| unittest | minimal | high | no |
| integration test | middium | middium | almost |
| e2e test | much  | low | yes |



Time consumed Running is easy to understand here. A larger scale of software ability a test is covering, the more time environment preparation and case running will consume us, therefore the testing efficiency is also lower. 
In terms of stability, the larger the case coverage scale is, the more problems it may encounter, and some of the probelems are not the real bugs we want to discover, but merely noises. In simulating real user behavior, only e2e can cover end-to-end to ensure that the entire link can work together.
​

As for the long-term value, it refers to the value of the existing case in the continuous software iteration process. For unittest, during code refactoring, it is adjusted with the adjustment of class and funciton, the code base is consistent with the hot spots in software iteration and continues to evolve. 
​

However, integration/e2e test is usually split into subsystem boundaries, whose external interfaces are relatively stable (there are very few functional changes during the software iteration of distributed systems, generally forward compatibility),integration/e2e test code base is relatively stable, which is very important in the future evolution of the system. It can timely discover whether new functions damage existing functions.
​

Combined with the characteristics of all three, the best way to balance is to comply with the pyramid model. The chassis is unittest, the middle is integration test, and the top layer is e2e.
​


             \                        
            / \                       
           /   \                      
          /     \                     
         /  e2e  \                    
        /----------                   
       /           \                  
      /intergeration\                 
     /               \                
    /-----------------\               
   /                   \              
  /      unit-test      \             
 /                       \            
---------------------------           

KubeVela would like to follow the 70/20/10 principle. that is, 70% unittest,20% integration test, and 10% e2e test. Each module has some differences. However, the higher the upper layer, the larger the test coverage, but the smaller the test case set. This pyramid model remains unchanged. The following situations need to be avoided:
​


-  Inverted pyramid, all rely on e2e to build the test
-  Funnel model, a large number of unit + e2e test, but no integration test
